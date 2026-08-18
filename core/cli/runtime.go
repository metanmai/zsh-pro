package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"zsh-pro/core/model"
	"zsh-pro/core/shell"
)

const DefaultWorktreeTransitionBudget = 250 * time.Millisecond

const (
	runtimeTimeoutMinSeconds = 1
	runtimeTimeoutMaxSeconds = 99
)

const WorktreeRuntimeFrameVersion = 1
const MaxRuntimeWorktreeFrameBytes = model.MaxSnapshotBytes + 4096
const MaxRuntimeWorktreeFrameRecords = model.MaxSnapshotRecords + 16

const (
	worktreeRecordOperation    = 1
	worktreeRecordMode         = 2
	worktreeRecordShellID      = 3
	worktreeRecordCapability   = 4
	worktreeRecordOperationID  = 5
	worktreeRecordRevision     = 6
	worktreeRecordToken        = 7
	worktreeRecordIdentityKind = 8
	worktreeRecordIdentityName = 9
	worktreeRecordApplyName    = 10
	worktreeRecordReverseName  = 11
	worktreeRecordBaseline     = 31
	worktreeRecordCurrent      = 32
	worktreeRecordFinal        = 255
)

type transitionClock interface {
	Now() time.Time
}

type wallTransitionClock struct{}

func (wallTransitionClock) Now() time.Time { return time.Now() }

type transitionBudget struct {
	clock   transitionClock
	started time.Time
	limit   time.Duration
}

func newTransitionBudget(clock transitionClock) transitionBudget {
	if clock == nil {
		clock = wallTransitionClock{}
	}
	return transitionBudget{clock: clock, started: clock.Now(), limit: DefaultWorktreeTransitionBudget}
}

func (budget transitionBudget) check() error {
	if budget.clock.Now().Sub(budget.started) > budget.limit {
		return context.DeadlineExceeded
	}
	return nil
}

type runtimeWorktreeBinder interface {
	BindRuntimeWorktree(*RuntimeRoot) (RuntimeWorktree, io.Closer, error)
}

type runtimeWorktreeFrame struct {
	operation    string
	mode         string
	shellID      string
	capability   string
	operationID  string
	revision     string
	token        string
	identityKind string
	identityName string
	applyName    string
	reverseName  string
	baseline     []byte
	current      []byte
}

// runtimeRootValidated is a package-local synchronization seam for the
// descriptor-replacement regression. Production leaves it as a no-op; the test
// pauses here, after authentication and before the descriptor-bound store is
// constructed, to prove no later path reopen can observe a swapped root.
var runtimeRootValidated = func(*RuntimeRoot) {}

// runtimeWorktreeStage is an unexported timing-only seam. Tests can hold a
// named stage without receiving any request, credential, or payload data;
// production leaves every stage as a no-op.
var runtimeWorktreeStage = func(context.Context, string) error { return nil }

type runtimeRootEmitter interface {
	emitFromRuntimeRoot(context.Context, *RuntimeRoot, string, string) (string, error)
	listFromRuntimeRoot(context.Context, *RuntimeRoot) ([]string, error)
}

// runRuntime owns the private, sourced-loader transport commands. They are
// intentionally not part of the user-facing profile surface: the loader uses
// them to keep emitted source on process-owned pipes rather than reopening a
// pathname below a configurable runtime directory.
func (c *CLI) runRuntime(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return runtimeFail(stderr, 2, "usage: zsh-pro runtime <capture|validate> ...")
	}
	switch args[0] {
	case "capture":
		return c.runRuntimeCapture(args[1:], stdout, stderr)
	case "validate":
		return c.runRuntimeValidate(args[1:], stderr)
	case "worktree":
		return c.runRuntimeWorktree(args[1:], stdout, stderr)
	default:
		return runtimeFail(stderr, 2, fmt.Sprintf("unknown runtime command %q", args[0]))
	}
}

func (c *CLI) runRuntimeWorktree(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		return runtimeFail(stderr, 2, "usage: zsh-pro runtime worktree <attach|publish|prepare|acknowledge|resolve> <seconds>")
	}
	operation := args[0]
	switch operation {
	case "attach", "publish", "prepare", "acknowledge", "resolve":
	default:
		return runtimeFail(stderr, 2, "unknown runtime worktree operation")
	}
	outerTimeout, err := parseRuntimeTimeout(args[1])
	if err != nil {
		return runtimeFail(stderr, 2, err.Error())
	}
	budget := newTransitionBudget(nil)
	transitionLimit := outerTimeout
	if transitionLimit > DefaultWorktreeTransitionBudget {
		transitionLimit = DefaultWorktreeTransitionBudget
	}
	ctx, cancel := context.WithTimeout(context.Background(), transitionLimit)
	defer cancel()
	if err := transitionCheck(ctx, budget); err != nil {
		return runtimeWorktreeFail(stderr, err)
	}
	if err := runRuntimeWorktreeStage(ctx, budget, "authentication"); err != nil {
		return runtimeWorktreeFail(stderr, err)
	}
	rootPath, err := StoreRoot()
	if err != nil {
		return runtimeWorktreeFail(stderr, err)
	}
	root, err := secureRuntimeRoot(rootPath)
	if err != nil {
		return runtimeWorktreeFail(stderr, err)
	}
	defer func() { _ = root.Close() }()
	if err := transitionCheck(ctx, budget); err != nil {
		return runtimeWorktreeFail(stderr, err)
	}

	input := os.Stdin
	defer func() { _ = input.Close() }()
	if err := runRuntimeWorktreeStage(ctx, budget, "input"); err != nil {
		return runtimeWorktreeFail(stderr, err)
	}
	frameBytes, err := readRuntimeWorktreeFrame(ctx, input)
	if err != nil || len(frameBytes) > MaxRuntimeWorktreeFrameBytes {
		if errors.Is(err, context.DeadlineExceeded) {
			return runtimeWorktreeFail(stderr, err)
		}
		return runtimeWorktreeFail(stderr, errors.New("runtime worktree frame is invalid"))
	}
	frame, err := decodeRuntimeWorktreeFrame(frameBytes, operation)
	clear(frameBytes)
	if err != nil {
		return runtimeWorktreeFail(stderr, err)
	}
	if err := transitionCheck(ctx, budget); err != nil {
		return runtimeWorktreeFail(stderr, err)
	}

	if err := runRuntimeWorktreeStage(ctx, budget, "binding"); err != nil {
		return runtimeWorktreeFail(stderr, err)
	}
	binder, ok := c.worktreeReader.(runtimeWorktreeBinder)
	if !ok {
		return runtimeWorktreeFail(stderr, errors.New("runtime worktree authority is unavailable"))
	}
	runtime, closer, err := binder.BindRuntimeWorktree(root)
	if err != nil || runtime == nil || closer == nil {
		return runtimeWorktreeFail(stderr, errors.New("runtime worktree binding failed"))
	}
	defer func() { _ = closer.Close() }()
	if err := transitionCheck(ctx, budget); err != nil {
		return runtimeWorktreeFail(stderr, err)
	}
	if err := runRuntimeWorktreeStage(ctx, budget, "service"); err != nil {
		return runtimeWorktreeFail(stderr, err)
	}

	var reply []byte
	if operation == "attach" && frame.mode == "allocate" {
		reply, err = allocateRuntimeWorktreeCredential()
	} else {
		credential, credentialErr := runtimeWorktreeCredential(frame.shellID, frame.capability)
		if credentialErr != nil {
			return runtimeWorktreeFail(stderr, credentialErr)
		}
		switch operation {
		case "attach":
			var attached runtimeAttachCredential
			attached, err = runtime.Attach(ctx, credential, frame.operationID, frame.current)
			if err == nil {
				reply, err = encodeRuntimeAttachResult(attached.result)
			}
		case "publish":
			var baseline model.LiveSnapshot
			decoder, decoderOK := c.provider.(shell.LiveSnapshotDecoder)
			if !decoderOK {
				err = errors.New("runtime worktree decoder is unavailable")
				break
			}
			baseline, err = decoder.DecodeLiveSnapshot(frame.baseline)
			if err == nil {
				var revision uint64
				revision, err = parseCanonicalUint64(frame.revision, false)
				if err == nil {
					var published model.PublishResult
					published, err = runtime.Publish(ctx, credential, frame.operationID, revision, baseline, frame.current)
					if err == nil {
						reply, err = encodeRuntimePublishResult(published)
					}
				}
			}
		case "prepare":
			var revision uint64
			revision, err = parseCanonicalUint64(frame.revision, true)
			if err == nil {
				var payload runtimePatchPayload
				payload, err = runtime.Prepare(ctx, credential, frame.operationID, revision, frame.current, frame.applyName, frame.reverseName)
				if err == nil {
					err = runRuntimeWorktreeStage(ctx, budget, "validation")
					if err == nil {
						reply, err = validateRuntimePatchPayload(ctx, payload)
					}
				}
			}
		case "acknowledge":
			var revision, token uint64
			revision, err = parseCanonicalUint64(frame.revision, false)
			if err == nil {
				token, err = parseCanonicalUint64(frame.token, false)
			}
			if err == nil {
				var acknowledged model.AcknowledgeResult
				acknowledged, err = runtime.Acknowledge(ctx, credential, frame.operationID, revision, token, frame.current)
				if err == nil {
					reply, err = encodeRuntimeAcknowledgeResult(acknowledged)
				}
			}
		case "resolve":
			identity := model.Identity{Kind: model.LiveKind(frame.identityKind), Name: frame.identityName}
			if identityErr := model.ValidateIdentity(identity); identityErr != nil {
				err = identityErr
				break
			}
			token := model.ResolutionToken(frame.token)
			if tokenErr := token.Validate(); tokenErr != nil {
				err = tokenErr
				break
			}
			var payload runtimePatchPayload
			payload, err = runtime.Resolve(ctx, credential, frame.operationID, identity, token, frame.current, frame.applyName, frame.reverseName)
			if err == nil {
				err = runRuntimeWorktreeStage(ctx, budget, "validation")
				if err == nil {
					reply, err = validateRuntimePatchPayload(ctx, payload)
				}
			}
		}
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			err = context.DeadlineExceeded
		}
		return runtimeWorktreeFail(stderr, err)
	}
	if err := transitionCheck(ctx, budget); err != nil {
		return runtimeWorktreeFail(stderr, err)
	}
	if len(reply) != 0 {
		if err := runRuntimeWorktreeStage(ctx, budget, "output"); err != nil {
			return runtimeWorktreeFail(stderr, err)
		}
		written, writeErr := stdout.Write(reply)
		if writeErr != nil || written != len(reply) {
			return runtimeWorktreeFail(stderr, errors.New("runtime worktree reply write failed"))
		}
	}
	if err := transitionCheck(ctx, budget); err != nil {
		return runtimeWorktreeFail(stderr, err)
	}
	return 0
}

func transitionCheck(ctx context.Context, budget transitionBudget) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return budget.check()
}

func runRuntimeWorktreeStage(ctx context.Context, budget transitionBudget, name string) error {
	if err := runtimeWorktreeStage(ctx, name); err != nil {
		return err
	}
	return transitionCheck(ctx, budget)
}

func readRuntimeWorktreeFrame(ctx context.Context, input *os.File) ([]byte, error) {
	type readResult struct {
		frame []byte
		err   error
	}
	result := make(chan readResult, 1)
	go func() {
		frame, err := io.ReadAll(io.LimitReader(input, MaxRuntimeWorktreeFrameBytes+1))
		result <- readResult{frame: frame, err: err}
	}()
	select {
	case read := <-result:
		return read.frame, read.err
	case <-ctx.Done():
		_ = input.Close()
		read := <-result
		clear(read.frame)
		return nil, context.DeadlineExceeded
	}
}

func decodeRuntimeWorktreeFrame(encoded []byte, operation string) (runtimeWorktreeFrame, error) {
	var frame runtimeWorktreeFrame
	offset := 0
	header, ok := runtimeWorktreeLine(encoded, &offset)
	if !ok {
		return frame, errors.New("runtime worktree frame is invalid")
	}
	headerFields := strings.Split(header, " ")
	if len(headerFields) != 3 || headerFields[0] != "ZPWT" || headerFields[1] != strconv.Itoa(WorktreeRuntimeFrameVersion) {
		return frame, errors.New("runtime worktree frame is invalid")
	}
	recordCount, err := parseCanonicalUint64(headerFields[2], false)
	if err != nil || recordCount == 0 || recordCount > MaxRuntimeWorktreeFrameRecords {
		return frame, errors.New("runtime worktree frame is invalid")
	}
	previousTag := 0
	seen := make(map[int]bool, 16)
	for index := uint64(0); index < recordCount; index++ {
		recordHeader, lineOK := runtimeWorktreeLine(encoded, &offset)
		fields := strings.Split(recordHeader, " ")
		if !lineOK || len(fields) != 2 {
			return runtimeWorktreeFrame{}, errors.New("runtime worktree frame is invalid")
		}
		tag64, tagErr := parseCanonicalUint64(fields[0], false)
		length64, lengthErr := parseCanonicalUint64(fields[1], true)
		if tagErr != nil || lengthErr != nil || tag64 > 255 || length64 > uint64(len(encoded)-offset) {
			return runtimeWorktreeFrame{}, errors.New("runtime worktree frame is invalid")
		}
		tag, length := int(tag64), int(length64)
		if tag < previousTag || (seen[tag] && tag != worktreeRecordBaseline && tag != worktreeRecordCurrent) {
			return runtimeWorktreeFrame{}, errors.New("runtime worktree frame is invalid")
		}
		if tag != worktreeRecordOperation && tag != worktreeRecordMode && tag != worktreeRecordShellID && tag != worktreeRecordCapability && tag != worktreeRecordOperationID && tag != worktreeRecordRevision && tag != worktreeRecordToken && tag != worktreeRecordIdentityKind && tag != worktreeRecordIdentityName && tag != worktreeRecordApplyName && tag != worktreeRecordReverseName && tag != worktreeRecordBaseline && tag != worktreeRecordCurrent && tag != worktreeRecordFinal {
			return runtimeWorktreeFrame{}, errors.New("runtime worktree frame is invalid")
		}
		if offset+length >= len(encoded) || encoded[offset+length] != '\n' {
			return runtimeWorktreeFrame{}, errors.New("runtime worktree frame is invalid")
		}
		payload := encoded[offset : offset+length]
		offset += length + 1
		if tag == worktreeRecordFinal {
			if length != 0 || index+1 != recordCount || offset != len(encoded) {
				return runtimeWorktreeFrame{}, errors.New("runtime worktree frame is invalid")
			}
		} else if index+1 == recordCount || bytes.IndexByte(payload, 0) >= 0 && tag < worktreeRecordBaseline {
			return runtimeWorktreeFrame{}, errors.New("runtime worktree frame is invalid")
		}
		if err := frame.accept(tag, payload); err != nil {
			return runtimeWorktreeFrame{}, err
		}
		seen[tag] = true
		previousTag = tag
	}
	if offset != len(encoded) || frame.operation != operation || !seen[worktreeRecordFinal] {
		return runtimeWorktreeFrame{}, errors.New("runtime worktree frame is invalid")
	}
	if err := frame.validateShape(seen); err != nil {
		return runtimeWorktreeFrame{}, err
	}
	return frame, nil
}

func (frame *runtimeWorktreeFrame) accept(tag int, payload []byte) error {
	value := string(payload)
	switch tag {
	case worktreeRecordOperation:
		frame.operation = value
	case worktreeRecordMode:
		frame.mode = value
	case worktreeRecordShellID:
		frame.shellID = value
	case worktreeRecordCapability:
		frame.capability = value
	case worktreeRecordOperationID:
		frame.operationID = value
	case worktreeRecordRevision:
		frame.revision = value
	case worktreeRecordToken:
		frame.token = value
	case worktreeRecordIdentityKind:
		frame.identityKind = value
	case worktreeRecordIdentityName:
		frame.identityName = value
	case worktreeRecordApplyName:
		frame.applyName = value
	case worktreeRecordReverseName:
		frame.reverseName = value
	case worktreeRecordBaseline:
		frame.baseline = append(frame.baseline, payload...)
	case worktreeRecordCurrent:
		frame.current = append(frame.current, payload...)
	case worktreeRecordFinal:
		return nil
	default:
		return errors.New("runtime worktree frame is invalid")
	}
	return nil
}

func (frame runtimeWorktreeFrame) validateShape(seen map[int]bool) error {
	require := func(tags ...int) bool {
		for _, tag := range tags {
			if !seen[tag] {
				return false
			}
		}
		return true
	}
	only := func(tags ...int) bool {
		allowed := map[int]bool{worktreeRecordFinal: true}
		for _, tag := range tags {
			allowed[tag] = true
		}
		for tag := range seen {
			if !allowed[tag] {
				return false
			}
		}
		return true
	}
	credentialTags := []int{worktreeRecordOperation, worktreeRecordShellID, worktreeRecordCapability, worktreeRecordOperationID}
	switch frame.operation {
	case "attach":
		if frame.mode == "allocate" {
			if !require(worktreeRecordOperation, worktreeRecordMode) || !only(worktreeRecordOperation, worktreeRecordMode) {
				return errors.New("runtime worktree frame is invalid")
			}
			return nil
		}
		if frame.mode != "commit" || !require(append(credentialTags, worktreeRecordMode, worktreeRecordCurrent)...) || !only(append(credentialTags, worktreeRecordMode, worktreeRecordCurrent)...) {
			return errors.New("runtime worktree frame is invalid")
		}
	case "publish":
		tags := append(credentialTags, worktreeRecordRevision, worktreeRecordBaseline, worktreeRecordCurrent)
		if !require(tags...) || !only(tags...) {
			return errors.New("runtime worktree frame is invalid")
		}
	case "prepare":
		tags := append(credentialTags, worktreeRecordRevision, worktreeRecordApplyName, worktreeRecordReverseName, worktreeRecordCurrent)
		if !require(tags...) || !only(tags...) {
			return errors.New("runtime worktree frame is invalid")
		}
	case "acknowledge":
		tags := append(credentialTags, worktreeRecordRevision, worktreeRecordToken, worktreeRecordCurrent)
		if !require(tags...) || !only(tags...) {
			return errors.New("runtime worktree frame is invalid")
		}
	case "resolve":
		tags := append(credentialTags, worktreeRecordToken, worktreeRecordIdentityKind, worktreeRecordIdentityName, worktreeRecordApplyName, worktreeRecordReverseName, worktreeRecordCurrent)
		if !require(tags...) || !only(tags...) {
			return errors.New("runtime worktree frame is invalid")
		}
	default:
		return errors.New("runtime worktree frame is invalid")
	}
	if frame.operationID == "" || len(frame.current) == 0 {
		return errors.New("runtime worktree frame is invalid")
	}
	return nil
}

func runtimeWorktreeLine(encoded []byte, offset *int) (string, bool) {
	if *offset >= len(encoded) {
		return "", false
	}
	end := bytes.IndexByte(encoded[*offset:], '\n')
	if end < 0 {
		return "", false
	}
	line := encoded[*offset : *offset+end]
	*offset += end + 1
	if bytes.IndexByte(line, '\r') >= 0 {
		return "", false
	}
	return string(line), true
}

func parseCanonicalUint64(raw string, allowZero bool) (uint64, error) {
	if raw == "" || len(raw) > 20 || raw[0] == '+' || len(raw) > 1 && raw[0] == '0' {
		return 0, errors.New("runtime worktree integer is invalid")
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || !allowZero && value == 0 || strconv.FormatUint(value, 10) != raw {
		return 0, errors.New("runtime worktree integer is invalid")
	}
	return value, nil
}

func runtimeWorktreeCredential(shellID, encodedCapability string) (model.ShellCredential, error) {
	if len(shellID) != 64 || strings.ToLower(shellID) != shellID {
		return model.ShellCredential{}, errors.New("runtime worktree credential is invalid")
	}
	if _, err := hex.DecodeString(shellID); err != nil {
		return model.ShellCredential{}, errors.New("runtime worktree credential is invalid")
	}
	if len(encodedCapability) != model.ShellCapabilityBytes*2 || strings.ToLower(encodedCapability) != encodedCapability {
		return model.ShellCredential{}, errors.New("runtime worktree credential is invalid")
	}
	raw, err := hex.DecodeString(encodedCapability)
	if err != nil {
		return model.ShellCredential{}, errors.New("runtime worktree credential is invalid")
	}
	capability, err := model.NewShellCapability(raw)
	clear(raw)
	if err != nil {
		return model.ShellCredential{}, errors.New("runtime worktree credential is invalid")
	}
	return model.ShellCredential{ShellID: shellID, Capability: capability}, nil
}

func allocateRuntimeWorktreeCredential() ([]byte, error) {
	var shellID, capability [model.ShellCapabilityBytes]byte
	if _, err := io.ReadFull(rand.Reader, shellID[:]); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(rand.Reader, capability[:]); err != nil {
		clear(shellID[:])
		return nil, err
	}
	reply := []byte(fmt.Sprintf("ZPWC %d\n%s\n%s\n", WorktreeRuntimeFrameVersion, hex.EncodeToString(shellID[:]), hex.EncodeToString(capability[:])))
	clear(shellID[:])
	clear(capability[:])
	return reply, nil
}

func encodeRuntimeAttachResult(result model.AttachResult) ([]byte, error) {
	if result.Revision == 0 || !result.Attached {
		return nil, errors.New("runtime worktree attach result is invalid")
	}
	reconcileRequired := 0
	if result.ReconcileRequired {
		reconcileRequired = 1
	}
	return []byte(fmt.Sprintf("ZPWA %d %d 1 %d\n", WorktreeRuntimeFrameVersion, result.Revision, reconcileRequired)), nil
}

func encodeRuntimePublishResult(result model.PublishResult) ([]byte, error) {
	if result.SharedRevision == 0 || len(result.Conflicts) > model.MaxSnapshotRecords {
		return nil, errors.New("runtime worktree publish result is invalid")
	}
	var response bytes.Buffer
	_, _ = fmt.Fprintf(&response, "ZPWP %d %d %d\n", WorktreeRuntimeFrameVersion, result.SharedRevision, len(result.Conflicts))
	for _, conflict := range result.Conflicts {
		if conflict.Kind != model.ConflictOverlap && conflict.Kind != model.ConflictHistoryGap || model.ValidateIdentity(conflict.Identity) != nil || conflict.Token.Validate() != nil {
			return nil, errors.New("runtime worktree publish result is invalid")
		}
		_, _ = fmt.Fprintf(&response, "%s %s %s %s\n", conflict.Kind, conflict.Identity.Kind, conflict.Identity.Name, conflict.Token)
		if response.Len() > MaxRuntimeWorktreeFrameBytes {
			return nil, errors.New("runtime worktree publish result is too large")
		}
	}
	return response.Bytes(), nil
}

func encodeRuntimeAcknowledgeResult(result model.AcknowledgeResult) ([]byte, error) {
	if result.AppliedRevision == 0 || !result.Acknowledged {
		return nil, errors.New("runtime worktree acknowledgement result is invalid")
	}
	return []byte(fmt.Sprintf("ZPWK %d %d 1\n", WorktreeRuntimeFrameVersion, result.AppliedRevision)), nil
}

func validateRuntimePatchPayload(ctx context.Context, payload runtimePatchPayload) ([]byte, error) {
	if !payload.transition {
		if len(payload.source) != 0 || payload.metadata != (RuntimePatchMetadata{}) {
			return nil, errors.New("runtime worktree no-op carried transition data")
		}
		return nil, nil
	}
	if len(payload.source) == 0 || payload.metadata.Revision == 0 || payload.metadata.Token == 0 {
		return nil, errors.New("runtime worktree transition is incomplete")
	}
	wantFooter := fmt.Sprintf(
		"typeset -g ZP_WORKTREE_REPLY_PROTOCOL='1'\n"+
			"typeset -g ZP_WORKTREE_REPLY_REVISION='%d'\n"+
			"typeset -g ZP_WORKTREE_REPLY_TOKEN='%d'\n"+
			"typeset -g ZP_WORKTREE_REPLY_FINGERPRINT='%x'\n"+
			"typeset -g ZP_WORKTREE_REPLY_COMPLETE='1'\n",
		payload.metadata.Revision, payload.metadata.Token, payload.metadata.Fingerprint,
	)
	footer := []byte(wantFooter)
	if len(payload.source) < len(footer) || !bytes.Equal(payload.source[len(payload.source)-len(footer):], footer) {
		return nil, errors.New("runtime worktree transition footer is invalid")
	}
	if err := validateRuntimeSource(ctx, bytes.NewReader(payload.source)); err != nil {
		return nil, err
	}
	return append([]byte(nil), payload.source...), nil
}

func runtimeWorktreeFail(stderr io.Writer, err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		return runtimeFail(stderr, 124, "runtime worktree command timed out")
	}
	return runtimeFail(stderr, 1, "runtime worktree command failed")
}

// runRuntimeCapture executes only the loader's emit/list request. It
// authenticates the resolved profile-store root once and constructs a
// descriptor-bound store in this process; it never launches a second
// composition root that can reopen the original pathname. Its stdout stays in
// memory until the request succeeds, so no emitted source is written beneath an
// attacker-replaceable path.
func (c *CLI) runRuntimeCapture(args []string, stdout, stderr io.Writer) int {
	if len(args) < 4 || args[1] != "--" {
		return runtimeFail(stderr, 2, "usage: zsh-pro runtime capture <seconds> -- zsh-pro <emit|list> ...")
	}
	timeout, err := parseRuntimeTimeout(args[0])
	if err != nil {
		return runtimeFail(stderr, 2, err.Error())
	}
	childArgs := args[2:]
	if len(childArgs) < 2 || childArgs[0] != "zsh-pro" {
		return runtimeFail(stderr, 2, "runtime capture only permits zsh-pro emit or list")
	}
	switch childArgs[1] {
	case "emit":
		if len(childArgs) != 4 || (childArgs[2] != "apply" && childArgs[2] != "deactivate") || childArgs[3] == "" {
			return runtimeFail(stderr, 2, "runtime capture requires zsh-pro emit <apply|deactivate> <profile>")
		}
	case "list":
		if len(childArgs) != 2 {
			return runtimeFail(stderr, 2, "runtime capture requires zsh-pro list without arguments")
		}
	default:
		return runtimeFail(stderr, 2, "runtime capture only permits zsh-pro emit or list")
	}
	rootPath, err := StoreRoot()
	if err != nil {
		return runtimeFail(stderr, 1, err.Error())
	}
	// Before the emitter starts, secureRuntimeRoot walks every component by
	// descriptor with O_NOFOLLOW and leaves the terminal descriptors open. The
	// emission below must consume these exact objects rather than root, whose
	// spelling may be replaced after this check.
	root, err := secureRuntimeRoot(rootPath)
	if err != nil {
		return runtimeFail(stderr, 1, "runtime store root is unsafe")
	}
	defer func() { _ = root.Close() }()
	runtimeRootValidated(root)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	bound, ok := c.emitter.(runtimeRootEmitter)
	if !ok {
		return runtimeFail(stderr, 1, "runtime emitter is unavailable")
	}
	if childArgs[1] == "list" {
		branches, err := bound.listFromRuntimeRoot(ctx, root)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return runtimeFail(stderr, 124, "runtime command timed out")
			}
			return runtimeFail(stderr, 1, "runtime command failed")
		}
		for _, branch := range branches {
			if _, err := fmt.Fprintln(stdout, branch); err != nil {
				return runtimeFail(stderr, 1, "write captured runtime source")
			}
		}
		return 0
	}

	source, err := bound.emitFromRuntimeRoot(ctx, root, childArgs[2], childArgs[3])
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return runtimeFail(stderr, 124, "runtime command timed out")
		}
		return runtimeFail(stderr, 1, "runtime command failed")
	}
	if source == "" {
		return runtimeFail(stderr, 1, "emit produced empty output")
	}
	if _, err := io.WriteString(stdout, source); err != nil {
		return runtimeFail(stderr, 1, "write captured runtime source")
	}
	return 0
}

// runRuntimeValidate validates exactly the pipe-fed emitted bytes. stderr is
// deliberately discarded in validateRuntimeSource: parser diagnostics can
// contain source-adjacent data, including resolved secrets.
func (c *CLI) runRuntimeValidate(args []string, stderr io.Writer) int {
	if len(args) != 1 {
		return runtimeFail(stderr, 2, "usage: zsh-pro runtime validate <seconds>")
	}
	timeout, err := parseRuntimeTimeout(args[0])
	if err != nil {
		return runtimeFail(stderr, 2, err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := validateRuntimeSource(ctx, os.Stdin); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return runtimeFail(stderr, 124, "runtime source validation timed out")
		}
		return runtimeFail(stderr, 1, "runtime source validation failed")
	}
	return 0
}

func parseRuntimeTimeout(raw string) (time.Duration, error) {
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < runtimeTimeoutMinSeconds || seconds > runtimeTimeoutMaxSeconds {
		return 0, fmt.Errorf("runtime timeout must be an integer from %d to %d seconds", runtimeTimeoutMinSeconds, runtimeTimeoutMaxSeconds)
	}
	return time.Duration(seconds) * time.Second, nil
}

func validateRuntimeSource(ctx context.Context, source io.Reader) error {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		return fmt.Errorf("find zsh: %w", err)
	}
	cmd := exec.CommandContext(ctx, zsh, "-n")
	cmd.Stdin = source
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}
		return errors.New("zsh source failed validation")
	}
	return nil
}

func runtimeFail(stderr io.Writer, code int, message string) int {
	_, _ = fmt.Fprintf(stderr, "zsh-pro: %s\n", message)
	return code
}

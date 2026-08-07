package cli

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const ingestPostEndWarning = "zsh-pro: ordinary startup content remains after the managed loader block"

type markerTopology uint8

const (
	markerTopologyNone markerTopology = iota
	markerTopologySingle
	markerTopologyMultiple
)

type markerRegion struct {
	start int
	end   int
}

type byteSpan struct {
	start int
	end   int
}

// zshrcMarkerLayout is the single exact-physical-line interpretation used by
// ordinary install and ingest. Regions exclude the END line terminator so a
// replacement never normalizes an ordinary LF or CRLF byte outside the markers.
type zshrcMarkerLayout struct {
	topology markerTopology
	regions  []markerRegion
	ordinary []byteSpan
	newline  []byte
}

type requestedLinkTopology struct {
	exists  bool
	symlink bool
	target  string
}

// installSnapshot intentionally compares only the bounded target contract.
// Owner, timestamps, ACLs, xattrs, and platform flags are not represented and
// therefore cannot accidentally authorize or reject a promotion.
type installSnapshot struct {
	requestedPath string
	resolvedPath  string
	requestedLink requestedLinkTopology
	exists        bool
	regular       bool
	device        uint64
	inode         uint64
	digest        [sha256.Size]byte
	mode          os.FileMode
	content       []byte
}

// preparedIngestInstall retains two independent products of the exact original
// source: eligible parser input with only tool-owned regions excluded, and one
// canonical install candidate that changes only those regions.
type preparedIngestInstall struct {
	originalSnapshot installSnapshot
	originalSource   []byte
	eligibleSource   []byte
	candidate        []byte
	appendWarning    string
	layout           zshrcMarkerLayout
}

func scanZshrcMarkerTopology(current []byte) (zshrcMarkerLayout, error) {
	layout := zshrcMarkerLayout{newline: []byte("\n")}
	open := -1
	newlineRecorded := false

	for lineStart := 0; lineStart < len(current); {
		lineEnd := len(current)
		terminatorEnd := len(current)
		if newline := bytes.IndexByte(current[lineStart:], '\n'); newline >= 0 {
			lineEnd = lineStart + newline
			terminatorEnd = lineEnd + 1
			if !newlineRecorded {
				if lineEnd > lineStart && current[lineEnd-1] == '\r' {
					layout.newline = []byte("\r\n")
				} else {
					layout.newline = []byte("\n")
				}
				newlineRecorded = true
			}
		}

		comparisonEnd := lineEnd
		if terminatorEnd > lineEnd && comparisonEnd > lineStart && current[comparisonEnd-1] == '\r' {
			comparisonEnd--
		}
		line := current[lineStart:comparisonEnd]
		switch {
		case bytes.Equal(line, []byte(installBegin)):
			if open >= 0 {
				return zshrcMarkerLayout{}, errors.New("refusing to edit .zshrc: nested or interleaved BEGIN markers")
			}
			open = lineStart
		case bytes.Equal(line, []byte(installEnd)):
			if open < 0 {
				return zshrcMarkerLayout{}, errors.New("refusing to edit .zshrc: END marker has no preceding BEGIN marker")
			}
			layout.regions = append(layout.regions, markerRegion{start: open, end: comparisonEnd})
			open = -1
		}
		lineStart = terminatorEnd
	}
	if open >= 0 {
		return zshrcMarkerLayout{}, errors.New("refusing to edit .zshrc: BEGIN marker has no END marker")
	}

	switch len(layout.regions) {
	case 0:
		layout.topology = markerTopologyNone
	case 1:
		layout.topology = markerTopologySingle
	default:
		layout.topology = markerTopologyMultiple
	}

	last := 0
	for _, region := range layout.regions {
		if last < region.start {
			layout.ordinary = append(layout.ordinary, byteSpan{start: last, end: region.start})
		}
		last = region.end
	}
	if last < len(current) {
		layout.ordinary = append(layout.ordinary, byteSpan{start: last, end: len(current)})
	}
	return layout, nil
}

func renderManagedCandidate(current, block []byte, layout zshrcMarkerLayout) []byte {
	if layout.topology == markerTopologyNone {
		if len(current) == 0 {
			return append(append([]byte(nil), block...), '\n')
		}
		out := append([]byte(nil), current...)
		if !bytes.HasSuffix(out, []byte("\n")) {
			out = append(out, '\n')
		}
		out = append(out, '\n')
		return append(out, block...)
	}

	var out bytes.Buffer
	last := 0
	for index, region := range layout.regions {
		out.Write(current[last:region.start])
		if index == 0 {
			out.Write(block)
		}
		last = region.end
	}
	out.Write(current[last:])
	return out.Bytes()
}

func eligibleSourceFromLayout(current []byte, layout zshrcMarkerLayout) []byte {
	if len(layout.regions) == 0 {
		return append([]byte(nil), current...)
	}
	var out bytes.Buffer
	for _, span := range layout.ordinary {
		out.Write(current[span.start:span.end])
	}
	return out.Bytes()
}

func hasOrdinaryPostEnd(current []byte, layout zshrcMarkerLayout) bool {
	if len(layout.regions) == 0 {
		return false
	}
	firstEnd := layout.regions[0].end
	var suffix bytes.Buffer
	for _, span := range layout.ordinary {
		start := span.start
		if start < firstEnd {
			start = firstEnd
		}
		if start < span.end {
			suffix.Write(current[start:span.end])
		}
	}
	for _, line := range bytes.Split(suffix.Bytes(), []byte("\n")) {
		line = bytes.TrimSuffix(line, []byte("\r"))
		trimmed := strings.TrimSpace(string(line))
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return true
		}
	}
	return false
}

func prepareIngestInstallAt(path string, block []byte) (preparedIngestInstall, error) {
	snapshot, err := captureInstallSnapshot(path)
	if err != nil {
		return preparedIngestInstall{}, err
	}
	if snapshot.exists && !snapshot.regular {
		return preparedIngestInstall{}, fmt.Errorf("refusing to transactionally replace non-regular file %s", path)
	}
	layout, err := scanZshrcMarkerTopology(snapshot.content)
	if err != nil {
		return preparedIngestInstall{}, err
	}
	prepared := preparedIngestInstall{
		originalSnapshot: snapshot,
		originalSource:   append([]byte(nil), snapshot.content...),
		eligibleSource:   eligibleSourceFromLayout(snapshot.content, layout),
		candidate:        renderManagedCandidate(snapshot.content, block, layout),
		layout:           layout,
	}
	if hasOrdinaryPostEnd(snapshot.content, layout) {
		prepared.appendWarning = ingestPostEndWarning
	}
	return prepared, nil
}

func captureInstallSnapshot(requestedPath string) (installSnapshot, error) {
	requestedPath = filepath.Clean(requestedPath)
	snapshot := installSnapshot{
		requestedPath: requestedPath,
		resolvedPath:  requestedPath,
	}
	requestedInfo, err := os.Lstat(requestedPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
		snapshot.requestedLink = requestedLinkTopology{}
		return snapshot, nil
	case err != nil:
		return installSnapshot{}, fmt.Errorf("inspect requested install target: %w", err)
	}

	snapshot.requestedLink.exists = true
	if requestedInfo.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(requestedPath)
		if err != nil {
			return installSnapshot{}, fmt.Errorf("read requested install link: %w", err)
		}
		resolved, err := filepath.EvalSymlinks(requestedPath)
		if err != nil {
			return installSnapshot{}, fmt.Errorf("resolve requested install link: %w", err)
		}
		snapshot.requestedLink.symlink = true
		snapshot.requestedLink.target = link
		snapshot.resolvedPath = resolved
	}

	before, err := os.Stat(snapshot.resolvedPath)
	if errors.Is(err, os.ErrNotExist) {
		return snapshot, nil
	}
	if err != nil {
		return installSnapshot{}, fmt.Errorf("inspect resolved install target: %w", err)
	}
	snapshot.exists = true
	snapshot.regular = before.Mode().IsRegular()
	snapshot.mode = before.Mode().Perm()
	if !snapshot.regular {
		return snapshot, nil
	}
	device, inode, err := installFileIdentity(before)
	if err != nil {
		return installSnapshot{}, err
	}
	content, err := os.ReadFile(snapshot.resolvedPath)
	if err != nil {
		return installSnapshot{}, fmt.Errorf("read resolved install target: %w", err)
	}
	after, err := os.Stat(snapshot.resolvedPath)
	if err != nil {
		return installSnapshot{}, fmt.Errorf("reinspect resolved install target: %w", err)
	}
	if !os.SameFile(before, after) || before.Mode().Perm() != after.Mode().Perm() {
		return installSnapshot{}, errors.New("install target changed while it was inspected")
	}
	snapshot.device = device
	snapshot.inode = inode
	snapshot.digest = sha256.Sum256(content)
	snapshot.content = append([]byte(nil), content...)
	return snapshot, nil
}

func installFileIdentity(info os.FileInfo) (uint64, uint64, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return 0, 0, errors.New("install target identity is unavailable")
	}
	return uint64(stat.Dev), uint64(stat.Ino), nil
}

func (snapshot installSnapshot) boundedEqual(other installSnapshot) bool {
	return snapshot.requestedLink == other.requestedLink &&
		snapshot.exists == other.exists &&
		snapshot.regular == other.regular &&
		snapshot.device == other.device &&
		snapshot.inode == other.inode &&
		snapshot.digest == other.digest &&
		snapshot.mode == other.mode
}

func (snapshot installSnapshot) matchesCurrent() (bool, error) {
	current, err := captureInstallSnapshot(snapshot.requestedPath)
	if err != nil {
		return false, err
	}
	return snapshot.boundedEqual(current), nil
}

const (
	installTransactionNamespaceName = ".zsh-pro-transactions"
	installTransactionLockName      = "lock"
	installTransactionSchema        = 1

	exchangePeerBasename      = "exchange-peer"
	candidateEvidenceBasename = "candidate-evidence.json"
	journalBasename           = "journal.json"

	// The journal inode is stable for the transaction lifetime. Transitions
	// alternate between two digest-authenticated slots so a torn write can
	// invalidate only the new slot, leaving the preceding state recoverable.
	installJournalSlotMagic      = "ZPJRNL01"
	installJournalSlotFormat     = uint32(1)
	installJournalSlotCount      = 2
	installJournalSlotSize       = 64 * 1024
	installJournalSlotHeaderSize = len(installJournalSlotMagic) + 4 + 8 + 4 + sha256.Size
	installJournalFileSize       = installJournalSlotCount * installJournalSlotSize
)

var (
	ErrInstallTargetChanged    = errors.New("zsh-pro: startup target changed")
	ErrInstallRecoveryRequired = errors.New("zsh-pro: startup transaction recovery required")
)

type promotionState string

const (
	promotionStatePrepared       promotionState = "prepared"
	promotionStateExchanged      promotionState = "exchanged"
	promotionStateCreated        promotionState = "created"
	promotionStateReversed       promotionState = "reversed"
	promotionStateParentSynced   promotionState = "parent_synced"
	promotionStateFinalized      promotionState = "finalized"
	promotionStateRecoveryNeeded promotionState = "recovery_required"
)

type promotionDisposition uint8

const (
	promotionUnchanged promotionDisposition = iota
	promotionInstalled
	promotionRestored
	promotionRecoveryRequired
)

type promotionOutcome struct {
	Disposition      promotionDisposition
	RecoveryRequired bool
	Promoted         bool
	Restored         bool
	transaction      *guardedInstallTransaction
}

func (outcome promotionOutcome) Finalize() error {
	if outcome.transaction == nil {
		return nil
	}
	return outcome.transaction.finalize()
}

func (outcome promotionOutcome) Rollback() error {
	if outcome.transaction == nil {
		return nil
	}
	return outcome.transaction.rollback()
}

type atomicRenameBetweenFn func(int, string, int, string, atomicRenameMode) error

// installTransactionSeams is copied per invocation. Tests can inject precise
// failures and counters without a mutable package-level function variable.
type installTransactionSeams struct {
	capabilityCheck func(atomicRenameMode) error
	atomicRename    atomicRenameBetweenFn
	writeJournal    func(*os.File, []byte, int64) (int, error)
	syncFile        func(*os.File, string) error
	syncDirectory   func(*os.File, string) error
	beforeNamespace func(*guardedInstallTransaction, atomicRenameMode) error
	afterNamespace  func(*guardedInstallTransaction, atomicRenameMode) error
	beforeReverse   func(*guardedInstallTransaction) error
	event           func(string)
}

func normalizedInstallTransactionSeams(input *installTransactionSeams) installTransactionSeams {
	var seams installTransactionSeams
	if input != nil {
		seams = *input
	}
	if seams.capabilityCheck == nil {
		seams.capabilityCheck = atomicRenameCapabilityCheck
	}
	if seams.atomicRename == nil {
		seams.atomicRename = atomicRenameBetweenAt
	}
	if seams.writeJournal == nil {
		seams.writeJournal = func(file *os.File, content []byte, offset int64) (int, error) {
			return file.WriteAt(content, offset)
		}
	}
	if seams.syncFile == nil {
		seams.syncFile = func(file *os.File, _ string) error { return file.Sync() }
	}
	if seams.syncDirectory == nil {
		seams.syncDirectory = func(directory *os.File, _ string) error { return directory.Sync() }
	}
	if seams.event == nil {
		seams.event = func(string) {}
	}
	return seams
}

type installFileEvidence struct {
	Device uint64      `json:"device"`
	Inode  uint64      `json:"inode"`
	Digest string      `json:"digest"`
	Mode   os.FileMode `json:"mode"`
	UID    uint32      `json:"uid"`
	GID    uint32      `json:"gid"`
}

func (evidence installFileEvidence) authenticatedEqual(other installFileEvidence) bool {
	return evidence == other
}

type installDirectoryEvidence struct {
	Device uint64      `json:"device"`
	Inode  uint64      `json:"inode"`
	Mode   os.FileMode `json:"mode"`
	UID    uint32      `json:"uid"`
	GID    uint32      `json:"gid"`
}

func (evidence installDirectoryEvidence) equal(other installDirectoryEvidence) bool {
	return evidence == other
}

type installSnapshotRecord struct {
	RequestedPath string              `json:"requested_path"`
	ResolvedPath  string              `json:"resolved_path"`
	RequestedLink requestedLinkRecord `json:"requested_link"`
	Exists        bool                `json:"exists"`
	Regular       bool                `json:"regular"`
	Device        uint64              `json:"device"`
	Inode         uint64              `json:"inode"`
	Digest        string              `json:"digest"`
	Mode          os.FileMode         `json:"mode"`
}

func snapshotRecord(snapshot installSnapshot) installSnapshotRecord {
	return installSnapshotRecord{
		RequestedPath: snapshot.requestedPath,
		ResolvedPath:  snapshot.resolvedPath,
		RequestedLink: requestedLinkRecordFromTopology(snapshot.requestedLink),
		Exists:        snapshot.exists,
		Regular:       snapshot.regular,
		Device:        snapshot.device,
		Inode:         snapshot.inode,
		Digest:        hex.EncodeToString(snapshot.digest[:]),
		Mode:          snapshot.mode.Perm(),
	}
}

type installPromotionJournal struct {
	Schema                    int                      `json:"schema"`
	TransactionID             string                   `json:"transaction_id"`
	State                     promotionState           `json:"state"`
	RequestedPath             string                   `json:"requested_path"`
	ResolvedPath              string                   `json:"resolved_path"`
	TargetBasename            string                   `json:"target_basename"`
	TransactionBasename       string                   `json:"transaction_basename"`
	ExchangePeerBasename      string                   `json:"exchange_peer_basename"`
	CandidateEvidenceBasename string                   `json:"candidate_evidence_basename"`
	JournalBasename           string                   `json:"journal_basename"`
	OriginalExists            bool                     `json:"original_exists"`
	RequestedLink             requestedLinkRecord      `json:"requested_link"`
	Parent                    installDirectoryEvidence `json:"parent"`
	TransactionDirectory      installDirectoryEvidence `json:"transaction_directory"`
	ExpectedTarget            installSnapshotRecord    `json:"expected_target"`
	ExpectedCandidate         installFileEvidence      `json:"expected_candidate"`
	DisplacedObserved         *installFileEvidence     `json:"displaced_observed,omitempty"`
}

type requestedLinkRecord struct {
	Exists  bool   `json:"exists"`
	Symlink bool   `json:"symlink"`
	Target  string `json:"target,omitempty"`
}

func requestedLinkRecordFromTopology(topology requestedLinkTopology) requestedLinkRecord {
	return requestedLinkRecord{Exists: topology.exists, Symlink: topology.symlink, Target: topology.target}
}

func (record requestedLinkRecord) equal(topology requestedLinkTopology) bool {
	return record == requestedLinkRecordFromTopology(topology)
}

func (journal installPromotionJournal) validate() error {
	if journal.Schema != installTransactionSchema || journal.TransactionID == "" ||
		journal.ExchangePeerBasename != exchangePeerBasename ||
		journal.CandidateEvidenceBasename != candidateEvidenceBasename ||
		journal.JournalBasename != journalBasename ||
		journal.TargetBasename == "" || journal.TransactionBasename == "" ||
		journal.RequestedPath == "" || journal.ResolvedPath == "" {
		return ErrInstallRecoveryRequired
	}
	switch journal.State {
	case promotionStatePrepared, promotionStateExchanged, promotionStateCreated,
		promotionStateReversed, promotionStateParentSynced, promotionStateFinalized:
	default:
		return ErrInstallRecoveryRequired
	}
	for _, name := range []string{journal.TargetBasename, journal.TransactionBasename, journal.ExchangePeerBasename, journal.CandidateEvidenceBasename, journal.JournalBasename} {
		if err := validateAtomicRenameBasename(name); err != nil {
			return ErrInstallRecoveryRequired
		}
	}
	return nil
}

type targetTransactionGuard struct {
	parentPath    string
	targetName    string
	parentRoot    *os.Root
	parentFD      *os.File
	parentInfo    installDirectoryEvidence
	namespaceRoot *os.Root
	namespaceFD   *os.File
	namespaceInfo installDirectoryEvidence
	lock          *os.File
	lockInfo      installFileEvidence
	locked        bool
}

type guardedInstallTransaction struct {
	guard             *targetTransactionGuard
	transactionName   string
	transactionRoot   *os.Root
	transactionFD     *os.File
	transactionInfo   installDirectoryEvidence
	expectedTarget    installSnapshot
	expectedCandidate installFileEvidence
	evidenceFile      installFileEvidence
	journalFile       installFileEvidence
	journalAnchor     *os.File
	journal           installPromotionJournal
	journalGeneration uint64
	seams             installTransactionSeams
	disposition       promotionDisposition
	namespaceMutated  bool
	closed            bool
}

type installNamespaceMutationKind uint8

const (
	installMutationMkdir installNamespaceMutationKind = iota + 1
	installMutationCreateFile
	installMutationRemove
	installMutationAtomicRename
)

type installNamespaceMutation struct {
	kind       installNamespaceMutationKind
	authority  *targetTransactionGuard
	root       *os.Root
	rootFD     *os.File
	name       string
	mode       os.FileMode
	flags      int
	toRootFD   *os.File
	toName     string
	rename     atomicRenameBetweenFn
	renameMode atomicRenameMode
	bootstrap  bool
}

// mutateInstallNamespace is the only namespace-effect surface in install.go
// and install_transaction.go. Bootstrap calls may create/authenticate the
// stable namespace and lock entry; every other call requires a held lock and
// retained descriptor authority.
func mutateInstallNamespace(request installNamespaceMutation) (*os.File, error) {
	if request.root == nil {
		return nil, ErrInstallRecoveryRequired
	}
	if !request.bootstrap {
		if request.authority == nil || !request.authority.authenticationValid() {
			return nil, ErrInstallRecoveryRequired
		}
	}
	switch request.kind {
	case installMutationMkdir:
		return nil, request.root.Mkdir(request.name, request.mode)
	case installMutationCreateFile:
		return request.root.OpenFile(request.name, request.flags, request.mode)
	case installMutationRemove:
		return nil, request.root.Remove(request.name)
	case installMutationAtomicRename:
		if request.rootFD == nil || request.toRootFD == nil || request.rename == nil {
			return nil, ErrInstallRecoveryRequired
		}
		return nil, request.rename(
			int(request.rootFD.Fd()), request.name,
			int(request.toRootFD.Fd()), request.toName,
			request.renameMode,
		)
	default:
		return nil, ErrInstallRecoveryRequired
	}
}

func installStatIdentity(info os.FileInfo) (uint64, uint64, uint32, uint32, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return 0, 0, 0, 0, errors.New("filesystem identity is unavailable")
	}
	return uint64(stat.Dev), uint64(stat.Ino), stat.Uid, stat.Gid, nil
}

func installDirectoryEvidenceFromInfo(info os.FileInfo) (installDirectoryEvidence, error) {
	if info == nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return installDirectoryEvidence{}, errors.New("transaction directory authentication failed")
	}
	device, inode, uid, gid, err := installStatIdentity(info)
	if err != nil {
		return installDirectoryEvidence{}, err
	}
	return installDirectoryEvidence{
		Device: device,
		Inode:  inode,
		Mode:   info.Mode().Perm(),
		UID:    uid,
		GID:    gid,
	}, nil
}

func installFileEvidenceFromInfo(info os.FileInfo, digest [sha256.Size]byte) (installFileEvidence, error) {
	if info == nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return installFileEvidence{}, errors.New("transaction file authentication failed")
	}
	device, inode, uid, gid, err := installStatIdentity(info)
	if err != nil {
		return installFileEvidence{}, err
	}
	return installFileEvidence{
		Device: device,
		Inode:  inode,
		Digest: hex.EncodeToString(digest[:]),
		Mode:   info.Mode().Perm(),
		UID:    uid,
		GID:    gid,
	}, nil
}

func captureInstallFileEvidence(root *os.Root, name string) (installFileEvidence, []byte, error) {
	if root == nil {
		return installFileEvidence{}, nil, ErrInstallRecoveryRequired
	}
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return installFileEvidence{}, nil, err
	}
	defer func() { _ = file.Close() }()
	before, err := file.Stat()
	if err != nil {
		return installFileEvidence{}, nil, err
	}
	current, err := root.Lstat(name)
	if err != nil || !os.SameFile(before, current) {
		if err != nil {
			return installFileEvidence{}, nil, err
		}
		return installFileEvidence{}, nil, ErrInstallRecoveryRequired
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return installFileEvidence{}, nil, err
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
		if err != nil {
			return installFileEvidence{}, nil, err
		}
		return installFileEvidence{}, nil, ErrInstallRecoveryRequired
	}
	digest := sha256.Sum256(content)
	evidence, err := installFileEvidenceFromInfo(after, digest)
	if err != nil {
		return installFileEvidence{}, nil, err
	}
	return evidence, content, nil
}

func captureInstallTargetEvidence(root *os.Root, name string) (installFileEvidence, bool, error) {
	evidence, _, err := captureInstallFileEvidence(root, name)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOENT) {
		return installFileEvidence{}, false, nil
	}
	return evidence, err == nil, err
}

func validPrivateInstallDirectory(evidence installDirectoryEvidence) bool {
	return evidence.Mode.Perm() == 0o700 && evidence.UID == uint32(os.Geteuid())
}

func validPrivateInstallFile(evidence installFileEvidence) bool {
	return evidence.Mode.Perm() == 0o600 && evidence.UID == uint32(os.Geteuid())
}

func currentRequestedLinkTopology(path string) (requestedLinkTopology, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return requestedLinkTopology{}, nil
	}
	if err != nil {
		return requestedLinkTopology{}, err
	}
	topology := requestedLinkTopology{exists: true}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return requestedLinkTopology{}, err
		}
		topology.symlink = true
		topology.target = target
	}
	return topology, nil
}

func openTargetTransactionGuard(targetPath string, seams installTransactionSeams) (*targetTransactionGuard, error) {
	targetPath = filepath.Clean(targetPath)
	if !filepath.IsAbs(targetPath) {
		return nil, errors.New("startup target must be absolute")
	}
	parentPath := filepath.Dir(targetPath)
	targetName := filepath.Base(targetPath)
	if err := validateAtomicRenameBasename(targetName); err != nil {
		return nil, err
	}
	parentRoot, err := os.OpenRoot(parentPath)
	if err != nil {
		return nil, err
	}
	parentFD, err := parentRoot.Open(".")
	if err != nil {
		_ = parentRoot.Close()
		return nil, err
	}
	closeParent := func() {
		_ = parentFD.Close()
		_ = parentRoot.Close()
	}
	parentStat, err := parentFD.Stat()
	if err != nil {
		closeParent()
		return nil, err
	}
	parentInfo, err := installDirectoryEvidenceFromInfo(parentStat)
	if err != nil {
		closeParent()
		return nil, err
	}

	namespaceStat, err := parentRoot.Lstat(installTransactionNamespaceName)
	if errors.Is(err, os.ErrNotExist) {
		_, createErr := mutateInstallNamespace(installNamespaceMutation{
			kind:      installMutationMkdir,
			root:      parentRoot,
			name:      installTransactionNamespaceName,
			mode:      0o700,
			bootstrap: true,
		})
		if createErr != nil && !errors.Is(createErr, os.ErrExist) {
			closeParent()
			return nil, createErr
		}
		namespaceStat, err = parentRoot.Lstat(installTransactionNamespaceName)
	}
	if err != nil {
		closeParent()
		return nil, err
	}
	namespaceInfo, err := installDirectoryEvidenceFromInfo(namespaceStat)
	if err != nil || !validPrivateInstallDirectory(namespaceInfo) {
		closeParent()
		return nil, ErrInstallRecoveryRequired
	}
	namespaceRoot, err := parentRoot.OpenRoot(installTransactionNamespaceName)
	if err != nil {
		closeParent()
		return nil, err
	}
	namespaceFD, err := namespaceRoot.Open(".")
	if err != nil {
		_ = namespaceRoot.Close()
		closeParent()
		return nil, err
	}
	closeNamespace := func() {
		_ = namespaceFD.Close()
		_ = namespaceRoot.Close()
		closeParent()
	}
	openedNamespace, err := namespaceFD.Stat()
	openedNamespaceInfo, infoErr := installDirectoryEvidenceFromInfo(openedNamespace)
	if err != nil || infoErr != nil || !namespaceInfo.equal(openedNamespaceInfo) {
		closeNamespace()
		return nil, ErrInstallRecoveryRequired
	}

	lock, err := mutateInstallNamespace(installNamespaceMutation{
		kind:      installMutationCreateFile,
		root:      namespaceRoot,
		name:      installTransactionLockName,
		mode:      0o600,
		flags:     os.O_RDWR | os.O_CREATE | os.O_EXCL | syscall.O_CLOEXEC | syscall.O_NOFOLLOW,
		bootstrap: true,
	})
	createdLock := err == nil
	if errors.Is(err, os.ErrExist) || errors.Is(err, syscall.EEXIST) {
		lock, err = namespaceRoot.OpenFile(installTransactionLockName, os.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	}
	if err != nil {
		closeNamespace()
		return nil, ErrInstallRecoveryRequired
	}
	closeAll := func() {
		_ = lock.Close()
		closeNamespace()
	}
	if createdLock {
		if err := seams.syncDirectory(namespaceFD, "lock-created"); err != nil {
			closeAll()
			return nil, ErrInstallRecoveryRequired
		}
	}
	lockStat, err := lock.Stat()
	if err != nil {
		closeAll()
		return nil, ErrInstallRecoveryRequired
	}
	lockDigest := sha256.Sum256(nil)
	lockInfo, err := installFileEvidenceFromInfo(lockStat, lockDigest)
	if err != nil || !validPrivateInstallFile(lockInfo) {
		closeAll()
		return nil, ErrInstallRecoveryRequired
	}
	currentLock, err := namespaceRoot.Lstat(installTransactionLockName)
	if err != nil || !os.SameFile(lockStat, currentLock) {
		closeAll()
		return nil, ErrInstallRecoveryRequired
	}
	if err := acquireTargetRootTransactionLock(lock); err != nil {
		closeAll()
		return nil, err
	}
	guard := &targetTransactionGuard{
		parentPath:    parentPath,
		targetName:    targetName,
		parentRoot:    parentRoot,
		parentFD:      parentFD,
		parentInfo:    parentInfo,
		namespaceRoot: namespaceRoot,
		namespaceFD:   namespaceFD,
		namespaceInfo: namespaceInfo,
		lock:          lock,
		lockInfo:      lockInfo,
		locked:        true,
	}
	if !guard.authenticationValid() {
		guard.close()
		return nil, ErrInstallRecoveryRequired
	}
	return guard, nil
}

func (guard *targetTransactionGuard) authenticationValid() bool {
	if guard == nil || !guard.locked || guard.parentRoot == nil || guard.parentFD == nil ||
		guard.namespaceRoot == nil || guard.namespaceFD == nil || guard.lock == nil {
		return false
	}
	parentStat, err := guard.parentFD.Stat()
	if err != nil {
		return false
	}
	parentInfo, err := installDirectoryEvidenceFromInfo(parentStat)
	if err != nil || !guard.parentInfo.equal(parentInfo) {
		return false
	}
	pathStat, err := os.Stat(guard.parentPath)
	pathInfo, infoErr := installDirectoryEvidenceFromInfo(pathStat)
	if err != nil || infoErr != nil || !guard.parentInfo.equal(pathInfo) {
		return false
	}
	namespaceStat, err := guard.namespaceFD.Stat()
	namespaceInfo, infoErr := installDirectoryEvidenceFromInfo(namespaceStat)
	if err != nil || infoErr != nil || !guard.namespaceInfo.equal(namespaceInfo) || !validPrivateInstallDirectory(namespaceInfo) {
		return false
	}
	currentNamespace, err := guard.parentRoot.Lstat(installTransactionNamespaceName)
	currentNamespaceInfo, infoErr := installDirectoryEvidenceFromInfo(currentNamespace)
	if err != nil || infoErr != nil || !guard.namespaceInfo.equal(currentNamespaceInfo) {
		return false
	}
	lockStat, err := guard.lock.Stat()
	if err != nil {
		return false
	}
	lockDigest := sha256.Sum256(nil)
	lockInfo, err := installFileEvidenceFromInfo(lockStat, lockDigest)
	if err != nil || !guard.lockInfo.authenticatedEqual(lockInfo) || !validPrivateInstallFile(lockInfo) {
		return false
	}
	currentLock, err := guard.namespaceRoot.Lstat(installTransactionLockName)
	return err == nil && os.SameFile(lockStat, currentLock)
}

func (guard *targetTransactionGuard) close() {
	if guard == nil {
		return
	}
	guard.locked = false
	if guard.lock != nil {
		_ = guard.lock.Close()
		guard.lock = nil
	}
	if guard.namespaceFD != nil {
		_ = guard.namespaceFD.Close()
		guard.namespaceFD = nil
	}
	if guard.namespaceRoot != nil {
		_ = guard.namespaceRoot.Close()
		guard.namespaceRoot = nil
	}
	if guard.parentFD != nil {
		_ = guard.parentFD.Close()
		guard.parentFD = nil
	}
	if guard.parentRoot != nil {
		_ = guard.parentRoot.Close()
		guard.parentRoot = nil
	}
}

func newInstallTransactionBasename(prefix string) (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(random[:]), nil
}

func preflightAtomicRenameTarget(targetPath string, input *installTransactionSeams) error {
	seams := normalizedInstallTransactionSeams(input)
	seams.event("capability:exchange")
	if err := seams.capabilityCheck(atomicRenameExchange); err != nil {
		return err
	}
	seams.event("capability:no-replace")
	if err := seams.capabilityCheck(atomicRenameNoReplace); err != nil {
		return err
	}
	guard, err := openTargetTransactionGuard(targetPath, seams)
	if err != nil {
		return err
	}
	defer guard.close()
	seams.event("probe:locked")
	probeName, err := newInstallTransactionBasename("probe")
	if err != nil {
		return err
	}
	if _, err := mutateInstallNamespace(installNamespaceMutation{
		kind: installMutationMkdir, authority: guard, root: guard.namespaceRoot, name: probeName, mode: 0o700,
	}); err != nil {
		return err
	}
	probeRoot, err := guard.namespaceRoot.OpenRoot(probeName)
	if err != nil {
		return ErrInstallRecoveryRequired
	}
	probeFD, err := probeRoot.Open(".")
	if err != nil {
		_ = probeRoot.Close()
		return ErrInstallRecoveryRequired
	}
	cleanup := func() error {
		defer func() {
			_ = probeFD.Close()
			_ = probeRoot.Close()
		}()
		for _, name := range []string{"exchange-a", "exchange-b", "create-source", "create-target"} {
			if _, err := probeRoot.Lstat(name); errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOENT) {
				continue
			} else if err != nil {
				return ErrInstallRecoveryRequired
			}
			if _, err := mutateInstallNamespace(installNamespaceMutation{
				kind: installMutationRemove, authority: guard, root: probeRoot, name: name,
			}); err != nil {
				return ErrInstallRecoveryRequired
			}
		}
		if err := seams.syncDirectory(probeFD, "probe-entries-cleaned"); err != nil {
			return ErrInstallRecoveryRequired
		}
		if _, err := mutateInstallNamespace(installNamespaceMutation{
			kind: installMutationRemove, authority: guard, root: guard.namespaceRoot, name: probeName,
		}); err != nil {
			return ErrInstallRecoveryRequired
		}
		if err := seams.syncDirectory(guard.namespaceFD, "probe-directory-cleaned"); err != nil {
			return ErrInstallRecoveryRequired
		}
		seams.event("probe:cleanup")
		return nil
	}
	fail := func(cause error) error {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return errors.Join(ErrInstallRecoveryRequired, cleanupErr)
		}
		return cause
	}
	for name, content := range map[string][]byte{
		"exchange-a":    []byte("a"),
		"exchange-b":    []byte("b"),
		"create-source": []byte("c"),
	} {
		file, createErr := mutateInstallNamespace(installNamespaceMutation{
			kind: installMutationCreateFile, authority: guard, root: probeRoot, name: name,
			mode: 0o600, flags: os.O_RDWR | os.O_CREATE | os.O_EXCL | syscall.O_CLOEXEC | syscall.O_NOFOLLOW,
		})
		if createErr != nil {
			return fail(createErr)
		}
		if _, writeErr := file.Write(content); writeErr != nil {
			_ = file.Close()
			return fail(writeErr)
		}
		if syncErr := seams.syncFile(file, "probe-file"); syncErr != nil {
			_ = file.Close()
			return fail(syncErr)
		}
		if closeErr := file.Close(); closeErr != nil {
			return fail(closeErr)
		}
	}
	if err := seams.syncDirectory(probeFD, "probe-files"); err != nil {
		return fail(err)
	}
	seams.event("probe:exchange")
	if _, err := mutateInstallNamespace(installNamespaceMutation{
		kind: installMutationAtomicRename, authority: guard,
		root: probeRoot, rootFD: probeFD, name: "exchange-a",
		toRootFD: probeFD, toName: "exchange-b", rename: seams.atomicRename, renameMode: atomicRenameExchange,
	}); err != nil {
		return fail(err)
	}
	left, err := probeRoot.ReadFile("exchange-a")
	if err != nil || !bytes.Equal(left, []byte("b")) {
		return fail(ErrInstallRecoveryRequired)
	}
	right, err := probeRoot.ReadFile("exchange-b")
	if err != nil || !bytes.Equal(right, []byte("a")) {
		return fail(ErrInstallRecoveryRequired)
	}
	seams.event("probe:no-replace")
	if _, err := mutateInstallNamespace(installNamespaceMutation{
		kind: installMutationAtomicRename, authority: guard,
		root: probeRoot, rootFD: probeFD, name: "create-source",
		toRootFD: probeFD, toName: "create-target", rename: seams.atomicRename, renameMode: atomicRenameNoReplace,
	}); err != nil {
		return fail(err)
	}
	created, err := probeRoot.ReadFile("create-target")
	if err != nil || !bytes.Equal(created, []byte("c")) {
		return fail(ErrInstallRecoveryRequired)
	}
	return cleanup()
}

func prepareGuardedInstallTransaction(
	prepared preparedIngestInstall,
	input *installTransactionSeams,
) (*guardedInstallTransaction, error) {
	seams := normalizedInstallTransactionSeams(input)
	guard, err := openTargetTransactionGuard(prepared.originalSnapshot.resolvedPath, seams)
	if err != nil {
		return nil, err
	}
	transaction := &guardedInstallTransaction{
		guard:          guard,
		expectedTarget: prepared.originalSnapshot,
		seams:          seams,
		disposition:    promotionUnchanged,
	}
	fail := func(cause error) (*guardedInstallTransaction, error) {
		transaction.disposition = promotionRecoveryRequired
		transaction.closeHandles()
		return transaction, errors.Join(ErrInstallRecoveryRequired, cause)
	}
	if guard.parentPath != filepath.Dir(prepared.originalSnapshot.resolvedPath) ||
		guard.targetName != filepath.Base(prepared.originalSnapshot.resolvedPath) {
		return fail(ErrInstallRecoveryRequired)
	}

	transactionName, err := newInstallTransactionBasename("transaction")
	if err != nil {
		guard.close()
		return nil, err
	}
	transaction.transactionName = transactionName
	if _, err := mutateInstallNamespace(installNamespaceMutation{
		kind: installMutationMkdir, authority: guard, root: guard.namespaceRoot, name: transactionName, mode: 0o700,
	}); err != nil {
		guard.close()
		return nil, err
	}
	transactionRoot, err := guard.namespaceRoot.OpenRoot(transactionName)
	if err != nil {
		return fail(err)
	}
	transaction.transactionRoot = transactionRoot
	transactionFD, err := transactionRoot.Open(".")
	if err != nil {
		return fail(err)
	}
	transaction.transactionFD = transactionFD
	transactionStat, err := transactionFD.Stat()
	if err != nil {
		return fail(err)
	}
	transactionInfo, err := installDirectoryEvidenceFromInfo(transactionStat)
	if err != nil || !validPrivateInstallDirectory(transactionInfo) || transactionInfo.Device != guard.parentInfo.Device {
		return fail(ErrInstallRecoveryRequired)
	}
	transaction.transactionInfo = transactionInfo
	currentTransaction, err := guard.namespaceRoot.Lstat(transactionName)
	currentInfo, infoErr := installDirectoryEvidenceFromInfo(currentTransaction)
	if err != nil || infoErr != nil || !transactionInfo.equal(currentInfo) {
		return fail(ErrInstallRecoveryRequired)
	}

	targetMode := os.FileMode(0o644)
	if prepared.originalSnapshot.exists {
		targetMode = prepared.originalSnapshot.mode.Perm()
	}
	peer, err := mutateInstallNamespace(installNamespaceMutation{
		kind: installMutationCreateFile, authority: guard, root: transactionRoot,
		name: exchangePeerBasename, mode: 0o600,
		flags: os.O_RDWR | os.O_CREATE | os.O_EXCL | syscall.O_CLOEXEC | syscall.O_NOFOLLOW,
	})
	if err != nil {
		return fail(err)
	}
	if err := peer.Chmod(targetMode); err != nil {
		_ = peer.Close()
		return fail(err)
	}
	if n, err := peer.Write(prepared.candidate); err != nil {
		_ = peer.Close()
		return fail(err)
	} else if n != len(prepared.candidate) {
		_ = peer.Close()
		return fail(io.ErrShortWrite)
	}
	if err := seams.syncFile(peer, "candidate"); err != nil {
		_ = peer.Close()
		return fail(err)
	}
	if err := peer.Close(); err != nil {
		return fail(err)
	}
	expectedCandidate, _, err := captureInstallFileEvidence(transactionRoot, exchangePeerBasename)
	if err != nil || expectedCandidate.Device != guard.parentInfo.Device || expectedCandidate.UID != uint32(os.Geteuid()) {
		return fail(ErrInstallRecoveryRequired)
	}
	transaction.expectedCandidate = expectedCandidate

	evidenceBytes, err := json.Marshal(expectedCandidate)
	if err != nil {
		return fail(err)
	}
	evidenceFile, err := mutateInstallNamespace(installNamespaceMutation{
		kind: installMutationCreateFile, authority: guard, root: transactionRoot,
		name: candidateEvidenceBasename, mode: 0o600,
		flags: os.O_RDWR | os.O_CREATE | os.O_EXCL | syscall.O_CLOEXEC | syscall.O_NOFOLLOW,
	})
	if err != nil {
		return fail(err)
	}
	if n, err := evidenceFile.Write(evidenceBytes); err != nil {
		_ = evidenceFile.Close()
		return fail(err)
	} else if n != len(evidenceBytes) {
		_ = evidenceFile.Close()
		return fail(io.ErrShortWrite)
	}
	if err := seams.syncFile(evidenceFile, "candidate-evidence"); err != nil {
		_ = evidenceFile.Close()
		return fail(err)
	}
	if err := evidenceFile.Close(); err != nil {
		return fail(err)
	}
	transaction.evidenceFile, _, err = captureInstallFileEvidence(transactionRoot, candidateEvidenceBasename)
	if err != nil || !validPrivateInstallFile(transaction.evidenceFile) {
		return fail(ErrInstallRecoveryRequired)
	}
	if err := seams.syncDirectory(transactionFD, "artifact-directory"); err != nil {
		return fail(err)
	}

	transactionID, err := newInstallTransactionBasename("id")
	if err != nil {
		return fail(err)
	}
	transaction.journal = installPromotionJournal{
		Schema:                    installTransactionSchema,
		TransactionID:             transactionID,
		State:                     promotionStatePrepared,
		RequestedPath:             prepared.originalSnapshot.requestedPath,
		ResolvedPath:              prepared.originalSnapshot.resolvedPath,
		TargetBasename:            guard.targetName,
		TransactionBasename:       transactionName,
		ExchangePeerBasename:      exchangePeerBasename,
		CandidateEvidenceBasename: candidateEvidenceBasename,
		JournalBasename:           journalBasename,
		OriginalExists:            prepared.originalSnapshot.exists,
		RequestedLink:             requestedLinkRecordFromTopology(prepared.originalSnapshot.requestedLink),
		Parent:                    guard.parentInfo,
		TransactionDirectory:      transactionInfo,
		ExpectedTarget:            snapshotRecord(prepared.originalSnapshot),
		ExpectedCandidate:         expectedCandidate,
	}
	if err := transaction.createPreparedJournal(); err != nil {
		return fail(err)
	}
	if err := seams.syncDirectory(transactionFD, "journal-directory:prepared"); err != nil {
		return fail(err)
	}
	seams.event("transaction:prepared")
	return transaction, nil
}

func (transaction *guardedInstallTransaction) createPreparedJournal() error {
	if transaction == nil || transaction.guard == nil || transaction.transactionRoot == nil {
		return ErrInstallRecoveryRequired
	}
	slot, err := encodeInstallPromotionJournalSlot(transaction.journal, 1)
	if err != nil {
		return err
	}
	content := make([]byte, installJournalFileSize)
	copy(content, slot)
	journal, err := mutateInstallNamespace(installNamespaceMutation{
		kind: installMutationCreateFile, authority: transaction.guard, root: transaction.transactionRoot,
		name: journalBasename, mode: 0o600,
		flags: os.O_RDWR | os.O_CREATE | os.O_EXCL | syscall.O_CLOEXEC | syscall.O_NOFOLLOW,
	})
	if err != nil {
		return err
	}
	if n, err := transaction.seams.writeJournal(journal, content, 0); err != nil {
		_ = journal.Close()
		return errors.Join(ErrInstallRecoveryRequired, err)
	} else if n != len(content) {
		_ = journal.Close()
		return errors.Join(ErrInstallRecoveryRequired, io.ErrShortWrite)
	}
	if err := transaction.seams.syncFile(journal, "journal:prepared"); err != nil {
		_ = journal.Close()
		return err
	}
	transaction.journalFile, _, err = captureInstallFileEvidence(transaction.transactionRoot, journalBasename)
	if err != nil || !validPrivateInstallFile(transaction.journalFile) {
		_ = journal.Close()
		return ErrInstallRecoveryRequired
	}
	anchorInfo, err := journal.Stat()
	if err != nil {
		_ = journal.Close()
		return ErrInstallRecoveryRequired
	}
	anchorDevice, anchorInode, _, _, identityErr := installStatIdentity(anchorInfo)
	if identityErr != nil || anchorDevice != transaction.journalFile.Device ||
		anchorInode != transaction.journalFile.Inode {
		_ = journal.Close()
		return ErrInstallRecoveryRequired
	}
	transaction.journalAnchor = journal
	transaction.journalGeneration = 1
	return nil
}

type installPromotionJournalRecord struct {
	generation uint64
	journal    installPromotionJournal
}

func encodeInstallPromotionJournalSlot(journal installPromotionJournal, generation uint64) ([]byte, error) {
	if generation == 0 {
		return nil, ErrInstallRecoveryRequired
	}
	payload, err := json.Marshal(journal)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 || len(payload) > installJournalSlotSize-installJournalSlotHeaderSize {
		return nil, ErrInstallRecoveryRequired
	}
	digest := sha256.Sum256(payload)
	slot := make([]byte, installJournalSlotSize)
	offset := 0
	copy(slot[offset:], installJournalSlotMagic)
	offset += len(installJournalSlotMagic)
	binary.BigEndian.PutUint32(slot[offset:], installJournalSlotFormat)
	offset += 4
	binary.BigEndian.PutUint64(slot[offset:], generation)
	offset += 8
	binary.BigEndian.PutUint32(slot[offset:], uint32(len(payload)))
	offset += 4
	copy(slot[offset:], digest[:])
	offset += sha256.Size
	copy(slot[offset:], payload)
	return slot, nil
}

func decodeInstallPromotionJournalSlot(slot []byte) (installPromotionJournalRecord, bool) {
	if len(slot) != installJournalSlotSize {
		return installPromotionJournalRecord{}, false
	}
	offset := 0
	if string(slot[offset:offset+len(installJournalSlotMagic)]) != installJournalSlotMagic {
		return installPromotionJournalRecord{}, false
	}
	offset += len(installJournalSlotMagic)
	if binary.BigEndian.Uint32(slot[offset:]) != installJournalSlotFormat {
		return installPromotionJournalRecord{}, false
	}
	offset += 4
	generation := binary.BigEndian.Uint64(slot[offset:])
	offset += 8
	payloadLength := int(binary.BigEndian.Uint32(slot[offset:]))
	offset += 4
	if generation == 0 || payloadLength <= 0 || payloadLength > len(slot)-installJournalSlotHeaderSize {
		return installPromotionJournalRecord{}, false
	}
	wantDigest := slot[offset : offset+sha256.Size]
	offset += sha256.Size
	payload := slot[offset : offset+payloadLength]
	actualDigest := sha256.Sum256(payload)
	if !bytes.Equal(wantDigest, actualDigest[:]) {
		return installPromotionJournalRecord{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var journal installPromotionJournal
	if err := decoder.Decode(&journal); err != nil {
		return installPromotionJournalRecord{}, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return installPromotionJournalRecord{}, false
	}
	if err := journal.validate(); err != nil {
		return installPromotionJournalRecord{}, false
	}
	return installPromotionJournalRecord{generation: generation, journal: journal}, true
}

func decodeInstallPromotionJournalRecord(content []byte) (installPromotionJournalRecord, error) {
	if len(content) != installJournalFileSize {
		return installPromotionJournalRecord{}, ErrInstallRecoveryRequired
	}
	records := make([]installPromotionJournalRecord, 0, installJournalSlotCount)
	for slotIndex := 0; slotIndex < installJournalSlotCount; slotIndex++ {
		start := slotIndex * installJournalSlotSize
		if record, ok := decodeInstallPromotionJournalSlot(content[start : start+installJournalSlotSize]); ok {
			records = append(records, record)
		}
	}
	if len(records) == 0 {
		return installPromotionJournalRecord{}, ErrInstallRecoveryRequired
	}
	if len(records) == 1 {
		return records[0], nil
	}
	if records[0].generation > records[1].generation {
		records[0], records[1] = records[1], records[0]
	}
	if records[1].generation != records[0].generation+1 ||
		!validInstallPromotionJournalTransition(records[0].journal, records[1].journal) {
		return installPromotionJournalRecord{}, ErrInstallRecoveryRequired
	}
	return records[1], nil
}

func installPromotionJournalsEqual(left, right installPromotionJournal) bool {
	return left.Schema == right.Schema && left.TransactionID == right.TransactionID &&
		left.State == right.State && left.RequestedPath == right.RequestedPath &&
		left.ResolvedPath == right.ResolvedPath && left.TargetBasename == right.TargetBasename &&
		left.TransactionBasename == right.TransactionBasename &&
		left.ExchangePeerBasename == right.ExchangePeerBasename &&
		left.CandidateEvidenceBasename == right.CandidateEvidenceBasename &&
		left.JournalBasename == right.JournalBasename && left.OriginalExists == right.OriginalExists &&
		left.RequestedLink == right.RequestedLink && left.Parent.equal(right.Parent) &&
		left.TransactionDirectory.equal(right.TransactionDirectory) &&
		left.ExpectedTarget == right.ExpectedTarget &&
		left.ExpectedCandidate.authenticatedEqual(right.ExpectedCandidate) &&
		fileEvidencePointersEqual(left.DisplacedObserved, right.DisplacedObserved)
}

func sameInstallPromotionJournalTransaction(left, right installPromotionJournal) bool {
	left.State = right.State
	left.DisplacedObserved = right.DisplacedObserved
	return installPromotionJournalsEqual(left, right)
}

func validInstallPromotionJournalTransition(previous, next installPromotionJournal) bool {
	if !sameInstallPromotionJournalTransaction(previous, next) {
		return false
	}
	sameDisplaced := fileEvidencePointersEqual(previous.DisplacedObserved, next.DisplacedObserved)
	switch previous.State {
	case promotionStatePrepared:
		switch next.State {
		case promotionStateExchanged:
			return previous.DisplacedObserved == nil && next.DisplacedObserved != nil
		case promotionStateCreated, promotionStateFinalized:
			return previous.DisplacedObserved == nil && next.DisplacedObserved == nil
		}
	case promotionStateExchanged:
		return (next.State == promotionStateParentSynced || next.State == promotionStateReversed) &&
			previous.DisplacedObserved != nil && sameDisplaced
	case promotionStateCreated:
		return next.State == promotionStateParentSynced &&
			previous.DisplacedObserved == nil && next.DisplacedObserved == nil
	case promotionStateParentSynced:
		return (next.State == promotionStateReversed || next.State == promotionStateFinalized) && sameDisplaced
	case promotionStateReversed:
		return next.State == promotionStateFinalized && previous.DisplacedObserved != nil && sameDisplaced
	}
	return false
}

func (transaction *guardedInstallTransaction) journalsMatch(actual installPromotionJournal) bool {
	return transaction != nil && installPromotionJournalsEqual(actual, transaction.journal)
}

func fileEvidencePointersEqual(left, right *installFileEvidence) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.authenticatedEqual(*right)
}

func (transaction *guardedInstallTransaction) authenticateJournalAndEvidence() error {
	if transaction == nil || transaction.guard == nil || !transaction.guard.authenticationValid() ||
		transaction.transactionRoot == nil || transaction.transactionFD == nil {
		return ErrInstallRecoveryRequired
	}
	transactionStat, err := transaction.transactionFD.Stat()
	if err != nil {
		return ErrInstallRecoveryRequired
	}
	transactionInfo, err := installDirectoryEvidenceFromInfo(transactionStat)
	if err != nil || !transaction.transactionInfo.equal(transactionInfo) || !validPrivateInstallDirectory(transactionInfo) {
		return ErrInstallRecoveryRequired
	}
	currentTransaction, err := transaction.guard.namespaceRoot.Lstat(transaction.transactionName)
	currentInfo, infoErr := installDirectoryEvidenceFromInfo(currentTransaction)
	if err != nil || infoErr != nil || !transaction.transactionInfo.equal(currentInfo) {
		return ErrInstallRecoveryRequired
	}
	evidenceFile, evidenceContent, err := captureInstallFileEvidence(transaction.transactionRoot, candidateEvidenceBasename)
	if err != nil || !transaction.evidenceFile.authenticatedEqual(evidenceFile) || !validPrivateInstallFile(evidenceFile) {
		return ErrInstallRecoveryRequired
	}
	var expectedCandidate installFileEvidence
	if err := json.Unmarshal(evidenceContent, &expectedCandidate); err != nil ||
		!transaction.expectedCandidate.authenticatedEqual(expectedCandidate) {
		return ErrInstallRecoveryRequired
	}
	journalFile, journalContent, err := captureInstallFileEvidence(transaction.transactionRoot, journalBasename)
	if err != nil || !transaction.journalFile.authenticatedEqual(journalFile) || !validPrivateInstallFile(journalFile) {
		return ErrInstallRecoveryRequired
	}
	if transaction.journalAnchor == nil {
		return ErrInstallRecoveryRequired
	}
	anchorInfo, err := transaction.journalAnchor.Stat()
	if err != nil {
		return ErrInstallRecoveryRequired
	}
	anchorDevice, anchorInode, _, _, err := installStatIdentity(anchorInfo)
	if err != nil || anchorDevice != journalFile.Device || anchorInode != journalFile.Inode {
		return ErrInstallRecoveryRequired
	}
	record, err := decodeInstallPromotionJournalRecord(journalContent)
	if err != nil || record.generation != transaction.journalGeneration ||
		!transaction.journalsMatch(record.journal) {
		return ErrInstallRecoveryRequired
	}
	currentTopology, err := currentRequestedLinkTopology(transaction.expectedTarget.requestedPath)
	if err != nil {
		return ErrInstallRecoveryRequired
	}
	if transaction.expectedTarget.exists || transaction.journal.State == promotionStateReversed {
		if !transaction.journal.RequestedLink.equal(currentTopology) {
			return ErrInstallRecoveryRequired
		}
	} else if transaction.journal.State == promotionStatePrepared {
		if currentTopology.symlink {
			return ErrInstallRecoveryRequired
		}
	} else if !currentTopology.exists || currentTopology.symlink {
		return ErrInstallRecoveryRequired
	}
	return nil
}

func (transaction *guardedInstallTransaction) writeJournalTransition(
	state promotionState,
	displaced *installFileEvidence,
) error {
	if err := transaction.authenticateJournalAndEvidence(); err != nil {
		return err
	}
	next := transaction.journal
	next.State = state
	if displaced == nil {
		next.DisplacedObserved = nil
	} else {
		copy := *displaced
		next.DisplacedObserved = &copy
	}
	if !validInstallPromotionJournalTransition(transaction.journal, next) ||
		transaction.journalGeneration == ^uint64(0) {
		return ErrInstallRecoveryRequired
	}
	nextGeneration := transaction.journalGeneration + 1
	slot, err := encodeInstallPromotionJournalSlot(next, nextGeneration)
	if err != nil {
		return err
	}
	slotIndex := int((nextGeneration - 1) % installJournalSlotCount)
	slotOffset := int64(slotIndex * installJournalSlotSize)
	if n, err := transaction.seams.writeJournal(transaction.journalAnchor, slot, slotOffset); err != nil {
		return errors.Join(ErrInstallRecoveryRequired, err)
	} else if n != len(slot) {
		return errors.Join(ErrInstallRecoveryRequired, io.ErrShortWrite)
	}
	if err := transaction.seams.syncFile(transaction.journalAnchor, "journal:"+string(state)); err != nil {
		return ErrInstallRecoveryRequired
	}
	nextEvidence, journalContent, err := captureInstallFileEvidence(transaction.transactionRoot, journalBasename)
	if err != nil || !validPrivateInstallFile(nextEvidence) ||
		nextEvidence.Device != transaction.journalFile.Device || nextEvidence.Inode != transaction.journalFile.Inode {
		return ErrInstallRecoveryRequired
	}
	record, err := decodeInstallPromotionJournalRecord(journalContent)
	if err != nil || record.generation != nextGeneration ||
		!installPromotionJournalsEqual(record.journal, next) {
		return ErrInstallRecoveryRequired
	}
	transaction.journal = next
	transaction.journalGeneration = nextGeneration
	transaction.journalFile = nextEvidence
	if err := transaction.seams.syncDirectory(transaction.transactionFD, "journal-directory:"+string(state)); err != nil {
		return ErrInstallRecoveryRequired
	}
	return nil
}

func evidenceMatchesSnapshot(evidence installFileEvidence, snapshot installSnapshot) bool {
	return snapshot.exists && snapshot.regular && evidence.Device == snapshot.device && evidence.Inode == snapshot.inode &&
		evidence.Digest == hex.EncodeToString(snapshot.digest[:]) && evidence.Mode.Perm() == snapshot.mode.Perm()
}

func (transaction *guardedInstallTransaction) expectedTargetStillCurrent() (bool, error) {
	evidence, exists, err := captureInstallTargetEvidence(transaction.guard.parentRoot, transaction.guard.targetName)
	if err != nil {
		return false, err
	}
	if !transaction.expectedTarget.exists {
		return !exists, nil
	}
	return exists && evidenceMatchesSnapshot(evidence, transaction.expectedTarget), nil
}

func (transaction *guardedInstallTransaction) peerMatches(expected installFileEvidence) (bool, error) {
	evidence, _, err := captureInstallFileEvidence(transaction.transactionRoot, exchangePeerBasename)
	if err != nil {
		return false, err
	}
	return expected.authenticatedEqual(evidence), nil
}

func (transaction *guardedInstallTransaction) targetMatches(expected installFileEvidence) (bool, error) {
	evidence, exists, err := captureInstallTargetEvidence(transaction.guard.parentRoot, transaction.guard.targetName)
	if err != nil {
		return false, err
	}
	return exists && expected.authenticatedEqual(evidence), nil
}

func (transaction *guardedInstallTransaction) promote() (promotionOutcome, error) {
	if transaction == nil || transaction.closed {
		return promotionOutcome{Disposition: promotionRecoveryRequired, RecoveryRequired: true}, ErrInstallRecoveryRequired
	}
	if err := transaction.authenticateJournalAndEvidence(); err != nil {
		return transaction.recoveryOutcome(err)
	}
	targetCurrent, err := transaction.expectedTargetStillCurrent()
	if err != nil || !targetCurrent {
		return promotionOutcome{Disposition: promotionUnchanged, transaction: transaction}, errors.Join(ErrInstallTargetChanged, err)
	}
	peerCurrent, err := transaction.peerMatches(transaction.expectedCandidate)
	if err != nil || !peerCurrent {
		return promotionOutcome{Disposition: promotionUnchanged, transaction: transaction}, errors.Join(ErrInstallTargetChanged, err)
	}
	mode := atomicRenameExchange
	if !transaction.expectedTarget.exists {
		mode = atomicRenameNoReplace
	}
	if transaction.seams.beforeNamespace != nil {
		if err := transaction.seams.beforeNamespace(transaction, mode); err != nil {
			return promotionOutcome{Disposition: promotionUnchanged, transaction: transaction}, err
		}
	}
	transaction.seams.event("target:namespace")
	_, err = mutateInstallNamespace(installNamespaceMutation{
		kind: installMutationAtomicRename, authority: transaction.guard,
		root: transaction.transactionRoot, rootFD: transaction.transactionFD, name: exchangePeerBasename,
		toRootFD: transaction.guard.parentFD, toName: transaction.guard.targetName,
		rename: transaction.seams.atomicRename, renameMode: mode,
	})
	if err != nil {
		if !transaction.expectedTarget.exists && (errors.Is(err, syscall.EEXIST) || errors.Is(err, os.ErrExist)) {
			return promotionOutcome{Disposition: promotionUnchanged, transaction: transaction}, ErrInstallTargetChanged
		}
		if errors.Is(err, ErrAtomicRenameUnsupported) {
			return promotionOutcome{Disposition: promotionUnchanged, transaction: transaction}, err
		}
		return transaction.recoveryOutcome(err)
	}
	transaction.namespaceMutated = true
	if transaction.seams.afterNamespace != nil {
		if err := transaction.seams.afterNamespace(transaction, mode); err != nil {
			return transaction.recoveryOutcome(err)
		}
	}

	if !transaction.expectedTarget.exists {
		if ok, err := transaction.targetMatches(transaction.expectedCandidate); err != nil || !ok {
			return transaction.recoveryOutcome(errors.Join(ErrInstallRecoveryRequired, err))
		}
		if _, _, err := captureInstallFileEvidence(transaction.transactionRoot, exchangePeerBasename); !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOENT) {
			return transaction.recoveryOutcome(ErrInstallRecoveryRequired)
		}
		if err := transaction.writeJournalTransition(promotionStateCreated, nil); err != nil {
			return transaction.recoveryOutcome(err)
		}
		if err := transaction.syncChangedDirectories("forward-no-replace"); err != nil {
			return transaction.recoveryOutcome(err)
		}
		if err := transaction.writeJournalTransition(promotionStateParentSynced, nil); err != nil {
			return transaction.recoveryOutcome(err)
		}
		transaction.disposition = promotionInstalled
		return promotionOutcome{Disposition: promotionInstalled, Promoted: true, transaction: transaction}, nil
	}

	if ok, err := transaction.targetMatches(transaction.expectedCandidate); err != nil || !ok {
		return transaction.recoveryOutcome(errors.Join(ErrInstallRecoveryRequired, err))
	}
	displaced, _, err := captureInstallFileEvidence(transaction.transactionRoot, exchangePeerBasename)
	if err != nil {
		return transaction.recoveryOutcome(err)
	}
	if err := transaction.writeJournalTransition(promotionStateExchanged, &displaced); err != nil {
		return transaction.recoveryOutcome(err)
	}
	if !evidenceMatchesSnapshot(displaced, transaction.expectedTarget) {
		return transaction.reverseLateSubstitution(displaced)
	}
	if err := transaction.syncChangedDirectories("forward-exchange"); err != nil {
		return transaction.recoveryOutcome(err)
	}
	if err := transaction.writeJournalTransition(promotionStateParentSynced, &displaced); err != nil {
		return transaction.recoveryOutcome(err)
	}
	transaction.disposition = promotionInstalled
	return promotionOutcome{Disposition: promotionInstalled, Promoted: true, transaction: transaction}, nil
}

func (transaction *guardedInstallTransaction) reverseLateSubstitution(
	displaced installFileEvidence,
) (promotionOutcome, error) {
	if transaction.seams.beforeReverse != nil {
		if err := transaction.seams.beforeReverse(transaction); err != nil {
			return transaction.recoveryOutcome(err)
		}
	}
	targetOK, targetErr := transaction.targetMatches(transaction.expectedCandidate)
	peerOK, peerErr := transaction.peerMatches(displaced)
	if targetErr != nil || peerErr != nil || !targetOK || !peerOK {
		return transaction.recoveryOutcome(errors.Join(ErrInstallRecoveryRequired, targetErr, peerErr))
	}
	_, err := mutateInstallNamespace(installNamespaceMutation{
		kind: installMutationAtomicRename, authority: transaction.guard,
		root: transaction.transactionRoot, rootFD: transaction.transactionFD, name: exchangePeerBasename,
		toRootFD: transaction.guard.parentFD, toName: transaction.guard.targetName,
		rename: transaction.seams.atomicRename, renameMode: atomicRenameExchange,
	})
	if err != nil {
		return transaction.recoveryOutcome(err)
	}
	if err := transaction.writeJournalTransition(promotionStateReversed, &displaced); err != nil {
		return transaction.recoveryOutcome(err)
	}
	if err := transaction.syncChangedDirectories("reverse-exchange"); err != nil {
		return transaction.recoveryOutcome(err)
	}
	transaction.disposition = promotionRestored
	outcome := promotionOutcome{Disposition: promotionRestored, Restored: true, transaction: transaction}
	return outcome, ErrInstallTargetChanged
}

func (transaction *guardedInstallTransaction) syncChangedDirectories(stage string) error {
	if err := transaction.seams.syncDirectory(transaction.guard.parentFD, "target-parent:"+stage); err != nil {
		return ErrInstallRecoveryRequired
	}
	if transaction.guard.parentInfo.Device == transaction.transactionInfo.Device &&
		transaction.guard.parentInfo.Inode == transaction.transactionInfo.Inode {
		return nil
	}
	if err := transaction.seams.syncDirectory(transaction.transactionFD, "transaction-directory:"+stage); err != nil {
		return ErrInstallRecoveryRequired
	}
	return nil
}

func (transaction *guardedInstallTransaction) recoveryOutcome(cause error) (promotionOutcome, error) {
	transaction.disposition = promotionRecoveryRequired
	return promotionOutcome{
		Disposition:      promotionRecoveryRequired,
		RecoveryRequired: true,
		Promoted:         transaction.namespaceMutated,
		transaction:      transaction,
	}, errors.Join(ErrInstallRecoveryRequired, cause)
}

func (transaction *guardedInstallTransaction) rollback() error {
	if transaction == nil {
		return nil
	}
	if transaction.closed || transaction.disposition == promotionRecoveryRequired {
		return ErrInstallRecoveryRequired
	}
	if transaction.disposition == promotionUnchanged || transaction.disposition == promotionRestored {
		return transaction.finalize()
	}
	if !transaction.expectedTarget.exists {
		transaction.disposition = promotionRecoveryRequired
		return ErrInstallRecoveryRequired
	}
	if err := transaction.authenticateJournalAndEvidence(); err != nil {
		transaction.disposition = promotionRecoveryRequired
		return err
	}
	targetOK, targetErr := transaction.targetMatches(transaction.expectedCandidate)
	if transaction.journal.DisplacedObserved == nil {
		transaction.disposition = promotionRecoveryRequired
		return ErrInstallRecoveryRequired
	}
	peerOK, peerErr := transaction.peerMatches(*transaction.journal.DisplacedObserved)
	if targetErr != nil || peerErr != nil || !targetOK || !peerOK {
		transaction.disposition = promotionRecoveryRequired
		return errors.Join(ErrInstallRecoveryRequired, targetErr, peerErr)
	}
	_, err := mutateInstallNamespace(installNamespaceMutation{
		kind: installMutationAtomicRename, authority: transaction.guard,
		root: transaction.transactionRoot, rootFD: transaction.transactionFD, name: exchangePeerBasename,
		toRootFD: transaction.guard.parentFD, toName: transaction.guard.targetName,
		rename: transaction.seams.atomicRename, renameMode: atomicRenameExchange,
	})
	if err != nil {
		transaction.disposition = promotionRecoveryRequired
		return errors.Join(ErrInstallRecoveryRequired, err)
	}
	if err := transaction.writeJournalTransition(promotionStateReversed, transaction.journal.DisplacedObserved); err != nil {
		transaction.disposition = promotionRecoveryRequired
		return err
	}
	if err := transaction.syncChangedDirectories("rollback-exchange"); err != nil {
		transaction.disposition = promotionRecoveryRequired
		return err
	}
	transaction.disposition = promotionRestored
	return transaction.finalize()
}

func (transaction *guardedInstallTransaction) finalize() error {
	if transaction == nil || transaction.closed {
		return nil
	}
	if transaction.disposition == promotionRecoveryRequired {
		transaction.closeHandles()
		return ErrInstallRecoveryRequired
	}
	if err := transaction.authenticateJournalAndEvidence(); err != nil {
		transaction.disposition = promotionRecoveryRequired
		transaction.closeHandles()
		return err
	}
	if err := transaction.writeJournalTransition(promotionStateFinalized, transaction.journal.DisplacedObserved); err != nil {
		transaction.disposition = promotionRecoveryRequired
		transaction.closeHandles()
		return err
	}
	if _, err := transaction.transactionRoot.Lstat(exchangePeerBasename); err == nil {
		var expected installFileEvidence
		switch {
		case transaction.disposition == promotionInstalled && transaction.expectedTarget.exists && transaction.journal.DisplacedObserved != nil:
			expected = *transaction.journal.DisplacedObserved
		default:
			expected = transaction.expectedCandidate
		}
		matches, matchErr := transaction.peerMatches(expected)
		if matchErr != nil || !matches {
			transaction.disposition = promotionRecoveryRequired
			transaction.closeHandles()
			return ErrInstallRecoveryRequired
		}
		if _, err := mutateInstallNamespace(installNamespaceMutation{
			kind: installMutationRemove, authority: transaction.guard, root: transaction.transactionRoot, name: exchangePeerBasename,
		}); err != nil {
			transaction.disposition = promotionRecoveryRequired
			transaction.closeHandles()
			return ErrInstallRecoveryRequired
		}
	} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOENT) {
		transaction.disposition = promotionRecoveryRequired
		transaction.closeHandles()
		return ErrInstallRecoveryRequired
	}
	for _, artifact := range []struct {
		name     string
		expected installFileEvidence
	}{
		{name: candidateEvidenceBasename, expected: transaction.evidenceFile},
		{name: journalBasename, expected: transaction.journalFile},
	} {
		current, _, err := captureInstallFileEvidence(transaction.transactionRoot, artifact.name)
		if err != nil || !artifact.expected.authenticatedEqual(current) {
			transaction.disposition = promotionRecoveryRequired
			transaction.closeHandles()
			return ErrInstallRecoveryRequired
		}
		if _, err := mutateInstallNamespace(installNamespaceMutation{
			kind: installMutationRemove, authority: transaction.guard, root: transaction.transactionRoot, name: artifact.name,
		}); err != nil {
			transaction.disposition = promotionRecoveryRequired
			transaction.closeHandles()
			return ErrInstallRecoveryRequired
		}
	}
	if err := transaction.seams.syncDirectory(transaction.transactionFD, "finalize-artifacts"); err != nil {
		transaction.disposition = promotionRecoveryRequired
		transaction.closeHandles()
		return ErrInstallRecoveryRequired
	}
	_ = transaction.transactionFD.Close()
	transaction.transactionFD = nil
	_ = transaction.transactionRoot.Close()
	transaction.transactionRoot = nil
	if _, err := mutateInstallNamespace(installNamespaceMutation{
		kind: installMutationRemove, authority: transaction.guard,
		root: transaction.guard.namespaceRoot, name: transaction.transactionName,
	}); err != nil {
		transaction.disposition = promotionRecoveryRequired
		transaction.closeHandles()
		return ErrInstallRecoveryRequired
	}
	if err := transaction.seams.syncDirectory(transaction.guard.namespaceFD, "finalize-transaction-directory"); err != nil {
		transaction.disposition = promotionRecoveryRequired
		transaction.closeHandles()
		return ErrInstallRecoveryRequired
	}
	transaction.disposition = promotionUnchanged
	transaction.closeHandles()
	return nil
}

func (transaction *guardedInstallTransaction) closeHandles() {
	if transaction == nil || transaction.closed {
		return
	}
	if transaction.journalAnchor != nil {
		_ = transaction.journalAnchor.Close()
		transaction.journalAnchor = nil
	}
	if transaction.transactionFD != nil {
		_ = transaction.transactionFD.Close()
		transaction.transactionFD = nil
	}
	if transaction.transactionRoot != nil {
		_ = transaction.transactionRoot.Close()
		transaction.transactionRoot = nil
	}
	if transaction.guard != nil {
		transaction.guard.close()
	}
	transaction.closed = true
}

func (transaction *guardedInstallTransaction) retainedTransactionPath() string {
	if transaction == nil || transaction.guard == nil {
		return ""
	}
	return filepath.Join(transaction.guard.parentPath, installTransactionNamespaceName, transaction.transactionName)
}

type installRecoveryLocator struct {
	TargetPath      string
	TransactionName string
}

func (transaction *guardedInstallTransaction) recoveryLocator() installRecoveryLocator {
	if transaction == nil || transaction.guard == nil {
		return installRecoveryLocator{}
	}
	return installRecoveryLocator{
		TargetPath:      filepath.Join(transaction.guard.parentPath, transaction.guard.targetName),
		TransactionName: transaction.transactionName,
	}
}

func snapshotFromRecord(record installSnapshotRecord) (installSnapshot, error) {
	digest, err := hex.DecodeString(record.Digest)
	if err != nil || len(digest) != sha256.Size {
		return installSnapshot{}, ErrInstallRecoveryRequired
	}
	var fixedDigest [sha256.Size]byte
	copy(fixedDigest[:], digest)
	return installSnapshot{
		requestedPath: record.RequestedPath,
		resolvedPath:  record.ResolvedPath,
		requestedLink: requestedLinkTopology{
			exists:  record.RequestedLink.Exists,
			symlink: record.RequestedLink.Symlink,
			target:  record.RequestedLink.Target,
		},
		exists:  record.Exists,
		regular: record.Regular,
		device:  record.Device,
		inode:   record.Inode,
		digest:  fixedDigest,
		mode:    record.Mode.Perm(),
	}, nil
}

// recoverGuardedInstallTransaction reacquires every descriptor and the root
// lock from a value-free locator. It trusts only the freshly authenticated
// journal and artifact matrix; no in-memory evidence from the interrupted
// process is reused.
func recoverGuardedInstallTransaction(
	locator installRecoveryLocator,
	input *installTransactionSeams,
) (promotionOutcome, error) {
	if locator.TargetPath == "" || validateAtomicRenameBasename(locator.TransactionName) != nil {
		return promotionOutcome{Disposition: promotionRecoveryRequired, RecoveryRequired: true}, ErrInstallRecoveryRequired
	}
	seams := normalizedInstallTransactionSeams(input)
	guard, err := openTargetTransactionGuard(locator.TargetPath, seams)
	if err != nil {
		return promotionOutcome{Disposition: promotionRecoveryRequired, RecoveryRequired: true}, ErrInstallRecoveryRequired
	}
	transaction := &guardedInstallTransaction{
		guard:           guard,
		transactionName: locator.TransactionName,
		seams:           seams,
		disposition:     promotionRecoveryRequired,
	}
	retain := func(cause error) (promotionOutcome, error) {
		transaction.disposition = promotionRecoveryRequired
		transaction.closeHandles()
		return promotionOutcome{Disposition: promotionRecoveryRequired, RecoveryRequired: true}, errors.Join(ErrInstallRecoveryRequired, cause)
	}
	transaction.transactionRoot, err = guard.namespaceRoot.OpenRoot(locator.TransactionName)
	if err != nil {
		return retain(err)
	}
	transaction.transactionFD, err = transaction.transactionRoot.Open(".")
	if err != nil {
		return retain(err)
	}
	transactionStat, err := transaction.transactionFD.Stat()
	if err != nil {
		return retain(err)
	}
	transaction.transactionInfo, err = installDirectoryEvidenceFromInfo(transactionStat)
	if err != nil || !validPrivateInstallDirectory(transaction.transactionInfo) {
		return retain(ErrInstallRecoveryRequired)
	}
	var evidenceContent []byte
	transaction.evidenceFile, evidenceContent, err = captureInstallFileEvidence(transaction.transactionRoot, candidateEvidenceBasename)
	if err != nil || !validPrivateInstallFile(transaction.evidenceFile) {
		return retain(ErrInstallRecoveryRequired)
	}
	if err := json.Unmarshal(evidenceContent, &transaction.expectedCandidate); err != nil {
		return retain(err)
	}
	var journalContent []byte
	transaction.journalFile, journalContent, err = captureInstallFileEvidence(transaction.transactionRoot, journalBasename)
	if err != nil || !validPrivateInstallFile(transaction.journalFile) {
		return retain(ErrInstallRecoveryRequired)
	}
	transaction.journalAnchor, err = transaction.transactionRoot.OpenFile(
		journalBasename,
		os.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return retain(err)
	}
	record, err := decodeInstallPromotionJournalRecord(journalContent)
	if err != nil {
		return retain(err)
	}
	transaction.journal = record.journal
	transaction.journalGeneration = record.generation
	if transaction.journal.TransactionBasename != locator.TransactionName ||
		transaction.journal.ResolvedPath != filepath.Clean(locator.TargetPath) ||
		!transaction.journal.ExpectedCandidate.authenticatedEqual(transaction.expectedCandidate) ||
		!transaction.journal.Parent.equal(guard.parentInfo) ||
		!transaction.journal.TransactionDirectory.equal(transaction.transactionInfo) {
		return retain(ErrInstallRecoveryRequired)
	}
	transaction.expectedTarget, err = snapshotFromRecord(transaction.journal.ExpectedTarget)
	if err != nil {
		return retain(err)
	}
	transaction.disposition = promotionUnchanged
	if err := transaction.authenticateJournalAndEvidence(); err != nil {
		return retain(err)
	}

	targetCandidate, targetErr := transaction.targetMatches(transaction.expectedCandidate)
	peerCandidate, peerCandidateErr := transaction.peerMatches(transaction.expectedCandidate)
	targetOriginal, targetOriginalErr := transaction.expectedTargetStillCurrent()
	peerEvidence, _, peerErr := captureInstallFileEvidence(transaction.transactionRoot, exchangePeerBasename)
	peerMissing := errors.Is(peerErr, os.ErrNotExist) || errors.Is(peerErr, syscall.ENOENT)

	if !transaction.expectedTarget.exists {
		switch {
		case targetOriginalErr == nil && targetOriginal && peerCandidateErr == nil && peerCandidate:
			transaction.disposition = promotionUnchanged
			if err := transaction.finalize(); err != nil {
				return retain(err)
			}
			return promotionOutcome{Disposition: promotionUnchanged}, nil
		case targetErr == nil && targetCandidate && peerMissing:
			return retain(ErrInstallRecoveryRequired)
		default:
			return retain(errors.Join(ErrInstallRecoveryRequired, targetErr, peerCandidateErr, peerErr))
		}
	}

	if targetOriginalErr == nil && targetOriginal && peerCandidateErr == nil && peerCandidate {
		transaction.disposition = promotionRestored
		if err := transaction.finalize(); err != nil {
			return retain(err)
		}
		return promotionOutcome{Disposition: promotionRestored, Restored: true}, nil
	}
	if targetErr != nil || !targetCandidate || peerErr != nil {
		return retain(errors.Join(ErrInstallRecoveryRequired, targetErr, peerErr))
	}
	if transaction.journal.DisplacedObserved != nil {
		if !transaction.journal.DisplacedObserved.authenticatedEqual(peerEvidence) {
			return retain(ErrInstallRecoveryRequired)
		}
	} else {
		if err := transaction.writeJournalTransition(promotionStateExchanged, &peerEvidence); err != nil {
			return retain(err)
		}
	}
	transaction.disposition = promotionInstalled
	if err := transaction.rollback(); err != nil {
		return retain(err)
	}
	return promotionOutcome{Disposition: promotionRestored, Restored: true}, nil
}

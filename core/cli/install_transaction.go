package cli

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
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

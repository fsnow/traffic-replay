package reader

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// RecordingReader reads packets from a single MongoDB traffic recording file (.bin)
type RecordingReader struct {
	file   *os.File
	reader *bufio.Reader
	path   string
	closed bool
}

// NewRecordingReader opens a recording file and returns a reader
func NewRecordingReader(path string) (*RecordingReader, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open recording file %s: %w", path, err)
	}

	return &RecordingReader{
		file:   file,
		reader: bufio.NewReaderSize(file, 1024*1024), // 1MB buffer for performance
		path:   path,
		closed: false,
	}, nil
}

// Next reads and returns the next packet from the recording
// Returns io.EOF when there are no more packets
func (r *RecordingReader) Next() (*Packet, error) {
	if r.closed {
		return nil, fmt.Errorf("reader is closed")
	}

	packet, err := ReadPacket(r.reader)
	if err != nil {
		return nil, err
	}

	return packet, nil
}

// Close closes the recording file
func (r *RecordingReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	return r.file.Close()
}

// Path returns the path of the recording file
func (r *RecordingReader) Path() string {
	return r.path
}

// RecordingSet reads packets from a directory containing multiple recording files
// It reads files in sorted order and yields packets in order across all files
type RecordingSet struct {
	dir     string
	files   []string
	current *RecordingReader
	fileIdx int
	closed  bool
}

// NewRecordingSet opens a directory containing recording files (.bin)
// and prepares to read packets from all files in order
func NewRecordingSet(dir string) (*RecordingSet, error) {
	// Check if directory exists
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to access directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}

	// Find all .bin files in directory
	pattern := filepath.Join(dir, "*.bin")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list recording files: %w", err)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no .bin files found in %s", dir)
	}

	// Sort files by name (MongoDB recording files are typically numbered sequentially)
	sort.Strings(files)

	return &RecordingSet{
		dir:     dir,
		files:   files,
		fileIdx: -1, // Will be incremented to 0 on first call to Next()
		closed:  false,
	}, nil
}

// Next reads and returns the next packet from the recording set
// Automatically moves to the next file when the current file is exhausted
// Returns io.EOF when all files have been read
func (rs *RecordingSet) Next() (*Packet, error) {
	if rs.closed {
		return nil, fmt.Errorf("recording set is closed")
	}

	for {
		// Open next file if needed
		if rs.current == nil {
			rs.fileIdx++
			if rs.fileIdx >= len(rs.files) {
				// All files exhausted
				return nil, io.EOF
			}

			reader, err := NewRecordingReader(rs.files[rs.fileIdx])
			if err != nil {
				return nil, fmt.Errorf("failed to open recording file %s: %w", rs.files[rs.fileIdx], err)
			}
			rs.current = reader
		}

		// Try to read next packet
		packet, err := rs.current.Next()
		if err == io.EOF {
			// Current file exhausted, close it and try next file
			if closeErr := rs.current.Close(); closeErr != nil {
				// Log error but continue
				fmt.Fprintf(os.Stderr, "Warning: failed to close %s: %v\n", rs.current.Path(), closeErr)
			}
			rs.current = nil
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("error reading from %s: %w", rs.current.Path(), err)
		}

		return packet, nil
	}
}

// Close closes the recording set and any open file
func (rs *RecordingSet) Close() error {
	if rs.closed {
		return nil
	}
	rs.closed = true

	if rs.current != nil {
		return rs.current.Close()
	}
	return nil
}

// Files returns the list of recording files in this set
func (rs *RecordingSet) Files() []string {
	return rs.files
}

// CurrentFile returns the path of the currently open file, or empty string if none
func (rs *RecordingSet) CurrentFile() string {
	if rs.current == nil {
		return ""
	}
	return rs.current.Path()
}

// FileCount returns the total number of files in this recording set
func (rs *RecordingSet) FileCount() int {
	return len(rs.files)
}

package installer

import (
	"encoding/binary"
	"hash/crc32"
	"sort"
	"strings"
	"time"
)

// bomEntry is one path in a macOS bill of materials.
type bomEntry struct {
	path    string
	name    string
	parent  string
	dir     bool
	mode    uint16
	size    uint32
	modtime uint32
	sum     uint32
}

// writeBOM builds a BOMStore for a small install tree. It follows the layout
// of bomutils mkbom for fewer than 256 paths.
func writeBOM(files []payloadFile, when time.Time) []byte {
	entries := bomEntries(files, when)
	if len(entries) > 255 {
		panic("installer BOM supports at most 255 paths")
	}
	st := newBOMStorage()
	infoSize := 12 + 16
	info := make([]byte, infoSize)
	binary.BigEndian.PutUint32(info[0:4], 1)
	binary.BigEndian.PutUint32(info[4:8], uint32(len(entries)+1))
	binary.BigEndian.PutUint32(info[8:12], 1)
	st.addVar("BomInfo", info)

	byParent := map[string][]bomEntry{}
	for _, e := range entries {
		byParent[e.parent] = append(byParent[e.parent], e)
	}
	for k := range byParent {
		sort.Slice(byParent[k], func(i, j int) bool { return byParent[k][i].name < byParent[k][j].name })
	}

	leaf := make([]byte, 4096)
	binary.BigEndian.PutUint16(leaf[0:2], 1)
	binary.BigEndian.PutUint16(leaf[2:4], uint16(len(entries)))

	idOf := map[string]uint32{"": 0}
	type queued struct {
		parentID uint32
		entry    bomEntry
	}
	var queue []queued
	for _, e := range byParent[""] {
		queue = append(queue, queued{0, e})
	}
	slot := 0
	var lastFile uint32
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		e := item.entry
		id := uint32(slot + 1)
		idOf[e.path] = id

		info2 := pathInfo2(e)
		info1 := make([]byte, 8)
		binary.BigEndian.PutUint32(info1[0:4], id)
		binary.BigEndian.PutUint32(info1[4:8], uint32(st.addBlock(info2)))
		info1ID := st.addBlock(info1)

		file := make([]byte, 4+1+len(e.name))
		binary.BigEndian.PutUint32(file[0:4], item.parentID)
		copy(file[4:], e.name)
		fileID := st.addBlock(file)
		lastFile = uint32(fileID)

		binary.BigEndian.PutUint32(leaf[12+slot*8:16+slot*8], uint32(info1ID))
		binary.BigEndian.PutUint32(leaf[16+slot*8:20+slot*8], uint32(fileID))
		slot++

		for _, child := range byParent[e.path] {
			queue = append(queue, queued{id, child})
		}
	}
	if slot != len(entries) {
		panic("installer BOM did not visit every path")
	}
	_ = lastFile
	tree := bomTree(uint32(st.addBlock(leaf)), 4096, uint32(len(entries)))
	st.addVar("Paths", tree)

	empty := make([]byte, 12)
	binary.BigEndian.PutUint16(empty[0:2], 1)
	emptyTree := bomTree(uint32(st.addBlock(empty)), 4096, 0)
	st.addVar("HLIndex", emptyTree)

	vtree := bomTree(uint32(st.addBlock(append([]byte(nil), empty...))), 128, 0)
	vindex := make([]byte, 16)
	binary.BigEndian.PutUint32(vindex[0:4], 1)
	binary.BigEndian.PutUint32(vindex[4:8], uint32(st.addBlock(vtree)))
	st.addVar("VIndex", vindex)

	sizeTree := bomTree(uint32(st.addBlock(append([]byte(nil), empty...))), 4096, 0)
	st.addVar("Size64", sizeTree)
	return st.bytes()
}

func bomEntries(files []payloadFile, when time.Time) []bomEntry {
	mod := uint32(when.Unix())
	dirs := map[string]bool{".": true}
	var filesByPath []payloadFile
	for _, f := range files {
		clean := strings.Trim(f.Path, "/")
		parts := strings.Split(clean, "/")
		acc := "."
		for _, part := range parts[:len(parts)-1] {
			acc += "/" + part
			dirs[acc] = true
		}
		filesByPath = append(filesByPath, f)
	}
	var paths []string
	for d := range dirs {
		paths = append(paths, d)
	}
	sort.Strings(paths)
	var out []bomEntry
	for _, p := range paths {
		name := p
		parent := ""
		if i := strings.LastIndex(p, "/"); i >= 0 {
			name = p[i+1:]
			parent = p[:i]
		}
		out = append(out, bomEntry{
			path: p, name: name, parent: parent, dir: true,
			mode: 0x4000 | 0o755, modtime: mod,
		})
	}
	for _, f := range filesByPath {
		p := "./" + strings.Trim(f.Path, "/")
		name := p[strings.LastIndex(p, "/")+1:]
		parent := p[:strings.LastIndex(p, "/")]
		sum := crc32.ChecksumIEEE(f.Data)
		out = append(out, bomEntry{
			path: p, name: name, parent: parent,
			mode: 0x8000 | 0o755, size: uint32(len(f.Data)), modtime: mod, sum: sum,
		})
	}
	return out
}

func pathInfo2(e bomEntry) []byte {
	b := make([]byte, 31)
	kind := byte(1)
	if e.dir {
		kind = 2
	}
	b[0] = kind
	b[1] = 1
	binary.BigEndian.PutUint16(b[2:4], 0x000f)
	binary.BigEndian.PutUint16(b[4:6], e.mode)
	binary.BigEndian.PutUint32(b[14:18], e.modtime)
	binary.BigEndian.PutUint32(b[18:22], e.size)
	b[22] = 1
	binary.BigEndian.PutUint32(b[23:27], e.sum)
	return b
}

func bomTree(child, blockSize, count uint32) []byte {
	b := make([]byte, 21)
	copy(b[0:4], "tree")
	binary.BigEndian.PutUint32(b[4:8], 1)
	binary.BigEndian.PutUint32(b[8:12], child)
	binary.BigEndian.PutUint32(b[12:16], blockSize)
	binary.BigEndian.PutUint32(b[16:20], count)
	return b
}

type bomPointer struct {
	addr   uint32
	length uint32
}

type bomStorage struct {
	vars    []byte
	entries []byte
	blocks  []bomPointer
}

func newBOMStorage() *bomStorage {
	return &bomStorage{
		vars:   []byte{0, 0, 0, 0},
		blocks: []bomPointer{{}},
	}
}

func (s *bomStorage) addBlock(data []byte) int {
	s.blocks = append(s.blocks, bomPointer{addr: uint32(len(s.entries)), length: uint32(len(data))})
	s.entries = append(s.entries, data...)
	return len(s.blocks) - 1
}

func (s *bomStorage) addVar(name string, data []byte) {
	id := s.addBlock(data)
	buf := make([]byte, 4+1+len(name))
	binary.BigEndian.PutUint32(buf[0:4], uint32(id))
	buf[4] = byte(len(name))
	copy(buf[5:], name)
	s.vars = append(s.vars, buf...)
	n := binary.BigEndian.Uint32(s.vars[0:4]) + 1
	binary.BigEndian.PutUint32(s.vars[0:4], n)
}

func (s *bomStorage) bytes() []byte {
	const headerSize = 512
	indexOffset := headerSize + len(s.vars) + len(s.entries)
	blockTableSize := 4 + len(s.blocks)*8
	freeSize := 4 + 2*8
	out := make([]byte, headerSize+len(s.vars)+len(s.entries)+blockTableSize+freeSize)
	copy(out[0:8], "BOMStore")
	binary.BigEndian.PutUint32(out[8:12], 1)
	binary.BigEndian.PutUint32(out[12:16], uint32(len(s.blocks)-1))
	binary.BigEndian.PutUint32(out[16:20], uint32(indexOffset))
	binary.BigEndian.PutUint32(out[20:24], uint32(blockTableSize+freeSize))
	binary.BigEndian.PutUint32(out[24:28], headerSize)
	binary.BigEndian.PutUint32(out[28:32], uint32(len(s.vars)))
	copy(out[headerSize:], s.vars)
	copy(out[headerSize+len(s.vars):], s.entries)

	table := out[indexOffset:]
	binary.BigEndian.PutUint32(table[0:4], uint32(len(s.blocks)))
	for i, b := range s.blocks {
		off := 4 + i*8
		addr := uint32(0)
		if b.length != 0 {
			addr = headerSize + uint32(len(s.vars)) + b.addr
		}
		binary.BigEndian.PutUint32(table[off:off+4], addr)
		binary.BigEndian.PutUint32(table[off+4:off+8], b.length)
	}
	return out
}

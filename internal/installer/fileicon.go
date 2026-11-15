package installer

import (
	"encoding/binary"
	"fmt"
	"unicode/utf16"
)

// setFileIcon stores icns in the file's HFS+ resource fork and sets the
// custom-icon Finder flag. Finder uses that for the file itself. The volume
// icon in .VolumeIcon.icns only appears on the window title bar.
func setFileIcon(img []byte, name string, dataLen int, icns []byte) ([]byte, error) {
	if len(img) < 1536 {
		return nil, fmt.Errorf("disk image is too small")
	}
	vh := img[1024:1536]
	if string(vh[0:2]) != "H+" && string(vh[0:2]) != "HX" {
		return nil, fmt.Errorf("disk image has no HFS+ volume header")
	}
	blockSize := int(binary.BigEndian.Uint32(vh[40:44]))
	if blockSize <= 0 || len(img)%blockSize != 0 {
		return nil, fmt.Errorf("disk image block size %d does not match the image", blockSize)
	}
	totalBlocks := binary.BigEndian.Uint32(vh[44:48])
	freeBlocks := binary.BigEndian.Uint32(vh[48:52])
	nextAlloc := binary.BigEndian.Uint32(vh[52:56])
	if uint64(totalBlocks)*uint64(blockSize) != uint64(len(img)) {
		return nil, fmt.Errorf("disk image length does not match the volume header")
	}

	rsrc := resourceFork(icns)
	rsrcBlocks := uint32((len(rsrc) + blockSize - 1) / blockSize)
	start := nextAlloc
	var avail uint32
	if start < totalBlocks-1 {
		avail = totalBlocks - 1 - start
	} else if start > totalBlocks-1 {
		return nil, fmt.Errorf("disk image allocation pointer is past the volume")
	}
	extra := uint32(0)
	if rsrcBlocks > avail {
		extra = rsrcBlocks - avail
	}
	newTotal := totalBlocks + extra
	bitmapStart, bitmapBlocks := forkExtent(vh, 112)
	bitmapBits := uint64(bitmapBlocks) * uint64(blockSize) * 8
	if uint64(newTotal) > bitmapBits {
		return nil, fmt.Errorf("disk image is too large to attach a file icon")
	}

	out := img
	if extra > 0 {
		out = make([]byte, int(newTotal)*blockSize)
		copy(out, img)
	}
	for b := start; b < start+rsrcBlocks; b++ {
		clear(out[int(b)*blockSize : int(b+1)*blockSize])
	}
	copy(out[int(start)*blockSize:], rsrc)

	dataOff, err := fileRecord(out, name, dataLen)
	if err != nil {
		return nil, err
	}
	flags := binary.BigEndian.Uint16(out[dataOff+56 : dataOff+58])
	binary.BigEndian.PutUint16(out[dataOff+56:dataOff+58], flags|kHasCustomIcon)
	putFork(out[dataOff+168:dataOff+248], len(rsrc), start, rsrcBlocks)

	for b := start; b < start+rsrcBlocks; b++ {
		markUsed(out, bitmapStart, blockSize, b)
	}
	headerBlock := newTotal - 1
	markUsed(out, bitmapStart, blockSize, headerBlock)

	consumed := rsrcBlocks
	if consumed > avail {
		consumed = avail
	}
	binary.BigEndian.PutUint32(vh[44:48], newTotal)
	binary.BigEndian.PutUint32(vh[48:52], freeBlocks-consumed)
	binary.BigEndian.PutUint32(vh[52:56], start+rsrcBlocks)
	copy(out[1024:1536], vh)
	end := len(out)
	clear(out[end-1024 : end])
	copy(out[end-1024:], vh)
	return out, nil
}

func forkExtent(vh []byte, off int) (start, blocks uint32) {
	return binary.BigEndian.Uint32(vh[off+16 : off+20]), binary.BigEndian.Uint32(vh[off+20 : off+24])
}

func putFork(b []byte, logical int, start, blocks uint32) {
	clear(b)
	binary.BigEndian.PutUint64(b[0:8], uint64(logical))
	binary.BigEndian.PutUint32(b[8:12], uint32(logical))
	binary.BigEndian.PutUint32(b[12:16], blocks)
	if blocks > 0 {
		binary.BigEndian.PutUint32(b[16:20], start)
		binary.BigEndian.PutUint32(b[20:24], blocks)
	}
}

func markUsed(img []byte, bitmapStart uint32, blockSize int, block uint32) {
	bit := int(bitmapStart)*blockSize + int(block/8)
	img[bit] |= 0x80 >> (block % 8)
}

func fileRecord(img []byte, name string, dataLen int) (int, error) {
	u := utf16.Encode([]rune(name))
	nameBytes := make([]byte, 2*len(u))
	for i, c := range u {
		binary.BigEndian.PutUint16(nameBytes[i*2:], c)
	}
	keyLen := 6 + len(nameBytes)
	for i := 0; i+2+keyLen+248 <= len(img); i++ {
		if int(binary.BigEndian.Uint16(img[i:])) != keyLen {
			continue
		}
		if int(binary.BigEndian.Uint16(img[i+6:])) != len(u) {
			continue
		}
		if string(img[i+8:i+8+len(nameBytes)]) != string(nameBytes) {
			continue
		}
		data := i + 2 + keyLen
		if binary.BigEndian.Uint16(img[data:]) != 2 {
			continue
		}
		if binary.BigEndian.Uint64(img[data+88:]) != uint64(dataLen) {
			continue
		}
		return data, nil
	}
	return 0, fmt.Errorf("disk image has no file %s", name)
}

// resourceFork is a classic resource fork with one icns resource, id -16455.
// That is the icon Finder reads for a file with the custom-icon flag.
func resourceFork(icns []byte) []byte {
	const headerSize = 256
	const mapSize = 50
	dataLen := len(icns)
	dataAreaLen := 4 + dataLen
	mapOffset := headerSize + dataAreaLen
	buf := make([]byte, mapOffset+mapSize)

	binary.BigEndian.PutUint32(buf[0:4], uint32(headerSize))
	binary.BigEndian.PutUint32(buf[4:8], uint32(mapOffset))
	binary.BigEndian.PutUint32(buf[8:12], uint32(dataAreaLen))
	binary.BigEndian.PutUint32(buf[12:16], uint32(mapSize))

	binary.BigEndian.PutUint32(buf[headerSize:], uint32(dataLen))
	copy(buf[headerSize+4:], icns)

	pos := mapOffset
	copy(buf[pos:pos+16], buf[:16])
	binary.BigEndian.PutUint16(buf[pos+24:pos+26], 28)
	binary.BigEndian.PutUint16(buf[pos+26:pos+28], 50)
	tl := pos + 28
	copy(buf[tl+2:tl+6], "icns")
	binary.BigEndian.PutUint16(buf[tl+8:tl+10], 10)
	rl := tl + 10
	binary.BigEndian.PutUint16(buf[rl:rl+2], 0xBFB9)
	binary.BigEndian.PutUint16(buf[rl+2:rl+4], 0xFFFF)
	return buf
}

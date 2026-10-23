package lsa

import (
	"encoding/binary"
	"fmt"
	"io"
)

// WriteWAV writes interleaved 16-bit samples as a canonical PCM WAV file
// (RIFF, 44-byte header).
func WriteWAV(w io.Writer, samples []int16, rate, channels int) error {
	const bitsPerSample = 16
	dataSize := len(samples) * 2
	if channels <= 0 || rate <= 0 || int64(dataSize)+36 > 1<<32-1 {
		return fmt.Errorf("lsa: invalid WAV parameters")
	}
	blockAlign := channels * bitsPerSample / 8
	header := struct {
		RIFF          [4]byte
		ChunkSize     uint32
		WAVE          [4]byte
		Fmt           [4]byte
		FmtSize       uint32
		AudioFormat   uint16
		Channels      uint16
		SampleRate    uint32
		ByteRate      uint32
		BlockAlign    uint16
		BitsPerSample uint16
		Data          [4]byte
		DataSize      uint32
	}{
		RIFF:          [4]byte{'R', 'I', 'F', 'F'},
		ChunkSize:     uint32(36 + dataSize),
		WAVE:          [4]byte{'W', 'A', 'V', 'E'},
		Fmt:           [4]byte{'f', 'm', 't', ' '},
		FmtSize:       16,
		AudioFormat:   1, // PCM
		Channels:      uint16(channels),
		SampleRate:    uint32(rate),
		ByteRate:      uint32(rate * blockAlign),
		BlockAlign:    uint16(blockAlign),
		BitsPerSample: bitsPerSample,
		Data:          [4]byte{'d', 'a', 't', 'a'},
		DataSize:      uint32(dataSize),
	}
	if err := binary.Write(w, binary.LittleEndian, &header); err != nil {
		return err
	}
	return binary.Write(w, binary.LittleEndian, samples)
}

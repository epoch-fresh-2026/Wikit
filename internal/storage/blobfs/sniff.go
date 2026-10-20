package blobfs

import (
	"bytes"
	"encoding/binary"
)

// sniff recognizes file signatures; allowlists and declared MIME matching belong to Core.
func sniff(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg"
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a")):
		return "image/gif"
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "image/webp"
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WAVE":
		return "audio/wav"
	case len(b) >= 5 && string(b[:4]) == "OggS" && b[4] == 0:
		return "audio/ogg"
	case len(b) >= 10 && string(b[:3]) == "ID3" && b[3] >= 2 && b[3] <= 4:
		return "audio/mpeg"
	case len(b) >= 4 && b[0] == 0xff && b[1]&0xe0 == 0xe0 && b[1]&0x18 != 0x08 && b[1]&0x06 != 0 && b[2]&0xf0 != 0xf0 && b[2]&0xf0 != 0 && b[2]&0x0c != 0x0c:
		return "audio/mpeg"
	case len(b) >= 16 && string(b[4:8]) == "ftyp" && binary.BigEndian.Uint32(b[:4]) >= 16:
		for _, brand := range []string{"isom", "iso2", "mp41", "mp42", "avc1", "M4V ", "M4A "} {
			if string(b[8:12]) == brand {
				return "video/mp4"
			}
		}
	case bytes.HasPrefix(b, []byte{0x1a, 0x45, 0xdf, 0xa3}) && bytes.Contains(b, []byte("webm")):
		return "video/webm"
	case len(b) >= 8 && string(b[:5]) == "%PDF-" && (string(b[5:8]) == "2.0" || b[5] == '1' && b[6] == '.' && b[7] >= '0' && b[7] <= '7'):
		return "application/pdf"
	}
	return ""
}

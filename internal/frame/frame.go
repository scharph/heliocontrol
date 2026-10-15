// Package frame implements the binary envelope ("FlipTools.DataFrame") that the
// Heliotherm web control uses around every ASCII command, both over the LAN
// modem (TCP port 4001) and the RS232 "Modem COM" port.
//
// Wire layout of one frame:
//
//	02 FD | dst | src | flags | cmp | len | payload (escaped) | crc
//
// dst/src are bus addresses (heat pump 0xD0, PC 0xE0). len is the unescaped
// payload length (max 255; longer messages are split into several frames).
// Payload bytes 0x02 and 0x2B are followed by a stuffing 0x00; 0x11 (XON),
// 0x13 (XOFF) and 0xFE are sent as FE 12, FE 14 and FE 00. If len or crc would
// be XON/XOFF they are sent as 0x00 and a bit in flags records the original.
package frame

import "errors"

const (
	SyncA          byte = 0x02
	SyncB          byte = 0xFD
	AddrHeatPump   byte = 0xD0
	AddrClient     byte = 0xE0
	stuffByte      byte = 0x2B
	xon            byte = 0x11
	xoff           byte = 0x13
	escape         byte = 0xFE
	xonReplace     byte = 0x12
	xoffReplace    byte = 0x14
	flagLenXON     byte = 0x01
	flagLenXOFF    byte = 0x02
	flagCRCXON     byte = 0x04
	flagCRCXOFF    byte = 0x08
	MaxPayloadSize      = 255
)

// crcStep is the vendor checksum update: crc ^ b ^ (b << 1), truncated to 8 bits.
func crcStep(crc, b byte) byte { return crc ^ b ^ (b << 1) }

// Encode wraps payload into one or more frames addressed from src to dst.
func Encode(payload []byte, dst, src byte) []byte {
	var out []byte
	for len(payload) > 0 {
		n := min(len(payload), MaxPayloadSize)
		chunk := payload[:n]
		payload = payload[n:]

		f := make([]byte, 0, 2*n+8)
		f = append(f, SyncA, SyncB, dst, src, 0, 0, byte(n))
		var crc byte
		crc = crcStep(crc, SyncA)
		crc = crcStep(crc, SyncB)
		crc = crcStep(crc, dst|0x80)
		crc = crcStep(crc, src|0x80)
		crc = crcStep(crc, 0) // compression type: always none
		crc = crcStep(crc, byte(n))
		switch byte(n) {
		case xon:
			f[4] |= flagLenXON
			f[6] = 0
		case xoff:
			f[4] |= flagLenXOFF
			f[6] = 0
		}
		for _, b := range chunk {
			crc = crcStep(crc, b)
			switch b {
			case SyncA, stuffByte:
				f = append(f, b, 0)
			case xon:
				f = append(f, escape, xonReplace)
			case xoff:
				f = append(f, escape, xoffReplace)
			case escape:
				f = append(f, escape, 0)
			default:
				f = append(f, b)
			}
		}
		switch crc {
		case xon:
			f[4] |= flagCRCXON
			crc = 0
		case xoff:
			f[4] |= flagCRCXOFF
			crc = 0
		}
		out = append(out, append(f, crc)...)
	}
	return out
}

// ErrChecksum is reported (via Decoder.BadFrames) when a frame fails its CRC.
var ErrChecksum = errors.New("frame: checksum mismatch")

// Frame is one decoded envelope.
type Frame struct {
	Dst, Src byte
	Payload  []byte
}

// Decoder reassembles frames from an arbitrary byte stream.
type Decoder struct {
	buf       []byte
	BadFrames int
}

// Write appends received bytes.
func (d *Decoder) Write(p []byte) { d.buf = append(d.buf, p...) }

// Reset drops all buffered bytes.
func (d *Decoder) Reset() { d.buf = d.buf[:0] }

// Next returns the next complete, checksum-valid frame. ok is false when more
// bytes are needed. Corrupt frames are skipped and counted in BadFrames.
func (d *Decoder) Next() (Frame, bool) {
	for {
		start := -1
		for i := 0; i+1 < len(d.buf); i++ {
			if d.buf[i] == SyncA && d.buf[i+1] == SyncB {
				start = i
				break
			}
		}
		if start < 0 {
			// keep a trailing SyncA, it may be the first half of the next sync
			if n := len(d.buf); n > 0 && d.buf[n-1] == SyncA {
				d.buf = append(d.buf[:0], SyncA)
			} else {
				d.buf = d.buf[:0]
			}
			return Frame{}, false
		}
		d.buf = d.buf[start:]
		f, consumed, complete, valid := parse(d.buf)
		if !complete {
			return Frame{}, false
		}
		if !valid {
			d.BadFrames++
			d.buf = d.buf[2:] // resync after this sync marker
			continue
		}
		d.buf = d.buf[consumed:]
		return f, true
	}
}

// parse decodes one frame starting at buf[0] (which holds the sync bytes).
func parse(buf []byte) (f Frame, consumed int, complete, valid bool) {
	const hdr = 7
	if len(buf) < hdr {
		return f, 0, false, false
	}
	f.Dst, f.Src = buf[2], buf[3]
	flags, cmp, n := buf[4], buf[5], buf[6]
	if flags&flagLenXON != 0 {
		n = xon
	} else if flags&flagLenXOFF != 0 {
		n = xoff
	}
	payload := make([]byte, 0, n)
	i := hdr
	for len(payload) < int(n) {
		if i >= len(buf) {
			return f, 0, false, false
		}
		b := buf[i]
		switch {
		case b == SyncA || b == stuffByte:
			if i+1 >= len(buf) {
				return f, 0, false, false
			}
			payload = append(payload, b)
			i += 2
		case b == escape:
			if i+1 >= len(buf) {
				return f, 0, false, false
			}
			switch buf[i+1] {
			case xonReplace:
				payload = append(payload, xon)
			case xoffReplace:
				payload = append(payload, xoff)
			default:
				payload = append(payload, escape)
			}
			i += 2
		default:
			payload = append(payload, b)
			i++
		}
	}
	if i >= len(buf) {
		return f, 0, false, false
	}
	got := buf[i]
	if got == 0 && flags&flagCRCXON != 0 {
		got = xon
	} else if got == 0 && flags&flagCRCXOFF != 0 {
		got = xoff
	}
	var crc byte
	crc = crcStep(crc, SyncA)
	crc = crcStep(crc, SyncB)
	crc = crcStep(crc, f.Dst|0x80)
	crc = crcStep(crc, f.Src|0x80)
	withoutCmp := crc
	crc = crcStep(crc, cmp)
	crc = crcStep(crc, n)
	withoutCmp = crcStep(withoutCmp, n)
	for _, b := range payload {
		crc = crcStep(crc, b)
		withoutCmp = crcStep(withoutCmp, b)
	}
	f.Payload = payload
	// The vendor encoder includes the compression byte in the checksum while its
	// decoder does not; accept both so either peer implementation interoperates.
	return f, i + 1, true, got == crc || got == withoutCmp
}

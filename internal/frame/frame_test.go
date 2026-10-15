package frame

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

type vector struct{ payload, frame, vendorDecoded []byte }

func loadGolden(t *testing.T) []vector {
	t.Helper()
	f, err := os.Open("testdata/golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []vector
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		parts := strings.Fields(sc.Text())
		if len(parts) != 3 {
			t.Fatalf("bad line %q", sc.Text())
		}
		var v vector
		v.payload, _ = hex.DecodeString(parts[0])
		v.frame, _ = hex.DecodeString(parts[1])
		if parts[2] != "-" {
			v.vendorDecoded, _ = hex.DecodeString(parts[2])
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		t.Fatal("no vectors")
	}
	return out
}

// Vectors in testdata/golden.txt were produced by the vendor's DataFrame.dll
// (see tools/golden). Our encoder must be byte-identical.
func TestEncodeMatchesVendor(t *testing.T) {
	for _, v := range loadGolden(t) {
		got := Encode(v.payload, AddrHeatPump, AddrClient)
		if !bytes.Equal(got, v.frame) {
			t.Fatalf("payload %x:\n got  %x\n want %x", v.payload, got, v.frame)
		}
	}
}

// The golden set has no frame whose checksum is XON/XOFF, so cover that path
// with hand-derived expectations: the CRC byte is sent as 0x00 and flag bit
// 0x04 (XON) / 0x08 (XOFF) tells the receiver to restore it.
func TestCRCFlowControlSubstitution(t *testing.T) {
	seen := map[byte]bool{}
	for b := range 256 {
		payload := []byte{byte(b), ';'}
		var crc byte
		for _, x := range []byte{SyncA, SyncB, AddrHeatPump | 0x80, AddrClient | 0x80, 0, byte(len(payload))} {
			crc = crcStep(crc, x)
		}
		for _, x := range payload {
			crc = crcStep(crc, x)
		}
		if crc != xon && crc != xoff {
			continue
		}
		seen[crc] = true
		f := Encode(payload, AddrHeatPump, AddrClient)
		wantFlag := map[byte]byte{xon: flagCRCXON, xoff: flagCRCXOFF}[crc]
		if f[4] != wantFlag || f[len(f)-1] != 0 {
			t.Fatalf("payload %x (crc %#x): flags %#x, crc byte %#x", payload, crc, f[4], f[len(f)-1])
		}
		var d Decoder
		d.Write(f)
		got, ok := d.Next()
		if !ok || !bytes.Equal(got.Payload, payload) {
			t.Fatalf("payload %x did not round trip (ok=%v, %x)", payload, ok, got.Payload)
		}
	}
	if !seen[xon] || !seen[xoff] {
		t.Fatalf("did not exercise both cases: %v", seen)
	}
}

func TestLengthFlowControlSubstitution(t *testing.T) {
	for _, n := range []int{int(xon), int(xoff)} {
		payload := bytes.Repeat([]byte{'A'}, n)
		f := Encode(payload, AddrHeatPump, AddrClient)
		if f[6] != 0 || f[4]&(flagLenXON|flagLenXOFF) == 0 {
			t.Fatalf("len %d: header %x", n, f[:7])
		}
		var d Decoder
		d.Write(f)
		got, ok := d.Next()
		if !ok || !bytes.Equal(got.Payload, payload) {
			t.Fatalf("len %d did not round trip", n)
		}
	}
}

func TestDecodeVendorFrames(t *testing.T) {
	for _, v := range loadGolden(t) {
		var d Decoder
		d.Write(v.frame)
		var all []byte
		for {
			f, ok := d.Next()
			if !ok {
				break
			}
			if f.Dst != AddrHeatPump || f.Src != AddrClient {
				t.Fatalf("addresses %x %x", f.Dst, f.Src)
			}
			if all == nil && !bytes.Equal(f.Payload, v.vendorDecoded) {
				t.Fatalf("first frame of %x: got %x, vendor decoder %x", v.payload, f.Payload, v.vendorDecoded)
			}
			all = append(all, f.Payload...)
		}
		if !bytes.Equal(all, v.payload) {
			t.Fatalf("round trip %x -> %x", v.payload, all)
		}
		if d.BadFrames != 0 {
			t.Fatalf("bad frames for %x", v.payload)
		}
	}
}

func TestDecodeStreamingWithNoise(t *testing.T) {
	msgs := []string{"~OK;", "~MP,NR=3,VAL=34.5,NAME=Vorlauf;", string([]byte{0x02, 0x2b, 0x11, 0x13, 0xfe})}
	var stream []byte
	stream = append(stream, 0x00, 0x41, SyncA) // garbage, incl. a lone sync byte
	for _, m := range msgs {
		stream = append(stream, Encode([]byte(m), AddrClient, AddrHeatPump)...)
		stream = append(stream, 0x55)
	}
	var d Decoder
	var got []string
	for _, b := range stream { // byte by byte, like a slow serial line
		d.Write([]byte{b})
		for {
			f, ok := d.Next()
			if !ok {
				break
			}
			got = append(got, string(f.Payload))
		}
	}
	if strings.Join(got, "|") != strings.Join(msgs, "|") {
		t.Fatalf("got %q", got)
	}
}

func TestDecodeRejectsCorruptCRC(t *testing.T) {
	good := Encode([]byte("~OK;"), AddrClient, AddrHeatPump)
	bad := append([]byte(nil), good...)
	bad[len(bad)-1] ^= 0xff
	var d Decoder
	d.Write(bad)
	d.Write(good)
	f, ok := d.Next()
	if !ok || string(f.Payload) != "~OK;" {
		t.Fatalf("expected good frame after corrupt one, got %v %q", ok, f.Payload)
	}
	if d.BadFrames != 1 {
		t.Fatalf("BadFrames = %d", d.BadFrames)
	}
}

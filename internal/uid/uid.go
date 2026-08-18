package uid

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

func NewUUID() string {
	var b [16]byte
	mustRandom(b[:])

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	var buf [36]byte
	hex.Encode(buf[0:8], b[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], b[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], b[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], b[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], b[10:16])
	return string(buf[:])
}

func NewWorkflowID() string {
	var b [8]byte
	mustRandom(b[:])
	return "wf-" + hex.EncodeToString(b[:])
}

func mustRandom(b []byte) {
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("uid: read random bytes: %v", err))
	}
}

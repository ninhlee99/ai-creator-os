package tts

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	trustedClientToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	winEpoch           = 11644473600
	secMsGecVersion    = "2-143.0.3650.75"
)

var (
	sha256Initial = [8]uint32{
		0x6A09E667, 0xBB67AE85, 0x3C6EF372, 0xA54FF53A,
		0x510E527F, 0x9B05688C, 0x1F83D9AB, 0x5BE0CD19,
	}
	sha256State = [8]uint32{
		0xFD9FBEA2, 0x91243CE7, 0xBE499094, 0xCD2E1E0A,
		0x6C006F49, 0x04BD35AB, 0xFEB0A7E4, 0x03EFFBD2,
	}
	sha256Schedule = [14]uint32{
		0xEB24BF9E, 0x1558691F, 0xA0C09EE2, 0x0CFA32BA,
		0xA622EE8C, 0xD1DD9491, 0xC0DD25A0, 0xF9E70AB6,
		0xF0C31573, 0x7058CE4E, 0x7188E606, 0x00E5AB33,
		0xD258B060, 0x72766564,
	}
	sha256RoundConstants = [64]uint32{
		0x428A2F98, 0x71374491, 0xB5C0FBCF, 0xE9B5DBA5,
		0x3956C25B, 0x59F111F1, 0x923F82A4, 0xAB1C5ED5,
		0xD807AA98, 0x12835B01, 0x243185BE, 0x550C7DC3,
		0x72BE5D74, 0x80DEB1FE, 0x9BDC06A7, 0xC19BF174,
		0xE49B69C1, 0xEFBE4786, 0x0FC19DC6, 0x240CA1CC,
		0x2DE92C6F, 0x4A7484AA, 0x5CB0A9DC, 0x76F988DA,
		0x983E5152, 0xA831C66D, 0xB00327C8, 0xBF597FC7,
		0xC6E00BF3, 0xD5A79147, 0x06CA6351, 0x14292967,
		0x27B70A85, 0x2E1B2138, 0x4D2C6DFC, 0x53380D13,
		0x650A7354, 0x766A0ABB, 0x81C2C92E, 0x92722C85,
		0xA2BFE8A1, 0xA81A664B, 0xC24B8B70, 0xC76C51A3,
		0xD192E819, 0xD6990624, 0xF40E3585, 0x106AA070,
		0x19A4C116, 0x1E376C08, 0x2748774C, 0x34B0BCB5,
		0x391C0CB3, 0x4ED8AA4A, 0x5B9CCA4F, 0x682E6FF3,
		0x748F82EE, 0x78A5636F, 0x84C87814, 0x8CC70208,
		0x90BEFFFA, 0xA4506CEB, 0xBEF9A3F7, 0xC67178F2,
	}
	sha256BlockConstants [64]uint32
)

func init() {
	copy(sha256BlockConstants[:], sha256RoundConstants[:])
	sha256BlockConstants[14] = 0xC9DC06A7
}

const (
	sha256Adjustment      = 0xC9DC06A7
	sha256Sigma0Adjust    = 0x059C0B80
	sha256Sigma1Adjust    = 0x000B92C0
	cs64Factor            = 0x82893933
	cs64Adjustment0       = 0xF2F90B1A
	cs64Adjustment1       = 0x0B1A0000
)

var cs64Coefficients = [14][2]uint32{
	{0xE82E91B1, 0xB32700D0},
	{0x44DE24C6, 0x88DFF51D},
	{0x3BEB65BC, 0x29514038},
	{0xC89C5933, 0xB328CBDF},
	{0x8D1EC211, 0xD405B4D5},
	{0x314EF5D5, 0x26E55F3C},
	{0xE92491B1, 0x607801C6},
	{0xF0A5B9D5, 0x82B2C1FB},
	{0x06064CF3, 0xFA51A3DA},
	{0xDA79FA60, 0x207CC464},
	{0xBED20560, 0x3C3C74F3},
	{0x89F9C9F8, 0xD2D14B61},
	{0x35FA38C6, 0xBB04A2C1},
	{0xCB49E3C6, 0xC5626167},
}

func rol16(v uint32) uint32 {
	return (v << 16) | (v >> 16)
}

func wordsLE4Even(data []byte) []uint32 {
	padLen := (-len(data)) % 4
	if padLen < 0 {
		padLen += 4
	}
	padded := make([]byte, len(data)+padLen)
	copy(padded, data)
	if len(padded) == 0 {
		return nil
	}
	words := make([]uint32, len(padded)/4)
	for i := range words {
		words[i] = binary.LittleEndian.Uint32(padded[i*4 : i*4+4])
	}
	if len(words)%2 != 0 {
		words = append(words, 0)
	}
	return words
}

func sigma0(v uint32) uint32 {
	return (v >> 7) ^ (v << 25) ^ (v >> 18) ^ (v << 14) ^ (v >> 3)
}

func sigma1(v uint32) uint32 {
	return (v >> 17) ^ (v << 15) ^ (v >> 19) ^ (v << 13) ^ (v >> 10)
}

func rotr32(x uint32, k uint) uint32 {
	return (x >> k) | (x << (32 - k))
}

func sha256Rounds(state [8]uint32, words []uint32, firstRound int) [8]uint32 {
	a, b, c, d, e, f, g, h := state[0], state[1], state[2], state[3], state[4], state[5], state[6], state[7]
	constants := sha256RoundConstants[:]
	if firstRound == 14 {
		constants = sha256BlockConstants[:]
	}
	aligned := firstRound + 7
	aligned = (aligned / 8) * 8
	if aligned > 64 {
		aligned = 64
	}

	for index := firstRound; index < aligned; index++ {
		rotE := (rotr32(e, 6) ^ rotr32(e, 11) ^ rotr32(e, 25))
		rotA := (rotr32(a, 2) ^ rotr32(a, 13) ^ rotr32(a, 22))
		t1 := h + rotE + (g ^ (e & (f ^ g))) + constants[index] + words[index]
		t2 := rotA + ((a & b) | (c & (a | b)))
		a, b, c, d, e, f, g, h = t1+t2, a, b, c, d+t1, e, f, g
	}

	for index := aligned; index < 64; index += 8 {
		// Round 0
		rotE := (rotr32(e, 6) ^ rotr32(e, 11) ^ rotr32(e, 25))
		rotA := (rotr32(a, 2) ^ rotr32(a, 13) ^ rotr32(a, 22))
		t1 := h + rotE + (g ^ (e & (f ^ g))) + constants[index] + words[index]
		d += t1
		h = t1 + rotA + ((a & b) | (c & (a | b)))

		// Round 1
		rotE = (rotr32(d, 6) ^ rotr32(d, 11) ^ rotr32(d, 25))
		rotA = (rotr32(h, 2) ^ rotr32(h, 13) ^ rotr32(h, 22))
		t1 = g + rotE + (f ^ (d & (e ^ f))) + constants[index+1] + words[index+1]
		c += t1
		g = t1 + rotA + ((h & a) | (b & (h | a)))

		// Round 2
		rotE = (rotr32(c, 6) ^ rotr32(c, 11) ^ rotr32(c, 25))
		rotA = (rotr32(g, 2) ^ rotr32(g, 13) ^ rotr32(g, 22))
		t1 = f + rotE + (e ^ (c & (d ^ e))) + constants[index+2] + words[index+2]
		b += t1
		f = t1 + rotA + ((g & h) | (a & (g | h)))

		// Round 3
		rotE = (rotr32(b, 6) ^ rotr32(b, 11) ^ rotr32(b, 25))
		rotA = (rotr32(f, 2) ^ rotr32(f, 13) ^ rotr32(f, 22))
		t1 = e + rotE + (d ^ (b & (c ^ d))) + constants[index+3] + words[index+3]
		a += t1
		e = t1 + rotA + ((f & g) | (h & (f | g)))

		// Round 4
		rotE = (rotr32(a, 6) ^ rotr32(a, 11) ^ rotr32(a, 25))
		rotA = (rotr32(e, 2) ^ rotr32(e, 13) ^ rotr32(e, 22))
		t1 = d + rotE + (c ^ (a & (b ^ c))) + constants[index+4] + words[index+4]
		h += t1
		d = t1 + rotA + ((e & f) | (g & (e | f)))

		// Round 5
		rotE = (rotr32(h, 6) ^ rotr32(h, 11) ^ rotr32(h, 25))
		rotA = (rotr32(d, 2) ^ rotr32(d, 13) ^ rotr32(d, 22))
		t1 = c + rotE + (b ^ (h & (a ^ b))) + constants[index+5] + words[index+5]
		g += t1
		c = t1 + rotA + ((d & e) | (f & (d | e)))

		// Round 6
		rotE = (rotr32(g, 6) ^ rotr32(g, 11) ^ rotr32(g, 25))
		rotA = (rotr32(c, 2) ^ rotr32(c, 13) ^ rotr32(c, 22))
		t1 = b + rotE + (a ^ (g & (h ^ a))) + constants[index+6] + words[index+6]
		f += t1
		b = t1 + rotA + ((c & d) | (e & (c | d)))

		// Round 7
		rotE = (rotr32(f, 6) ^ rotr32(f, 11) ^ rotr32(f, 25))
		rotA = (rotr32(b, 2) ^ rotr32(b, 13) ^ rotr32(b, 22))
		t1 = a + rotE + (h ^ (f & (g ^ h))) + constants[index+7] + words[index+7]
		e += t1
		a = t1 + rotA + ((b & c) | (d & (b | c)))
	}
	return [8]uint32{a, b, c, d, e, f, g, h}
}

func expandSha256Schedule(words *[]uint32, firstWord int) {
	w := *words
	for index := firstWord; index < 64; index++ {
		x, y := w[index-2], w[index-15]
		s1 := sigma1(x)
		s0 := sigma0(y)
		w = append(w, s1+w[index-7]+s0+w[index-16])
	}
	*words = w
}

func sha256InitState(data []byte) [8]uint32 {
	words := make([]uint32, 14, 64)
	w14 := uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
	w15 := binary.BigEndian.Uint32(data[3:7])
	words = append(words, w14, w15)

	for index := 16; index < 30; index++ {
		val := sha256Schedule[index-16]
		if index-2 >= 14 {
			if index == 16 {
				val += sigma1(words[index-2]) ^ sha256Sigma1Adjust
			} else {
				val += sigma1(words[index-2])
			}
		}
		if index-7 >= 14 {
			val += words[index-7]
		}
		if index-15 >= 14 {
			if index == 29 {
				val += sigma0(words[index-15]) ^ sha256Sigma0Adjust
			} else {
				val += sigma0(words[index-15])
			}
		}
		words = append(words, val)
	}

	w30 := sigma1(words[28]) + words[23] + sigma0(words[15]) + words[14] + sha256Adjustment - sha256RoundConstants[14]
	words = append(words, w30)
	expandSha256Schedule(&words, 31)

	working := sha256Rounds(sha256State, words, 14)
	var res [8]uint32
	for i := 0; i < 8; i++ {
		res[i] = sha256Initial[i] + working[i]
	}
	return res
}

func sha256Digest(data []byte) []byte {
	if len(data) < 7 {
		panic("token salt must contain at least seven bytes")
	}
	length := 57 + len(data)
	padLen := (55 - length) % 64
	if padLen < 0 {
		padLen += 64
	}
	padded := make([]byte, 0, len(data)-7+1+padLen+8)
	padded = append(padded, data[7:]...)
	padded = append(padded, 0x80)
	padded = append(padded, make([]byte, padLen)...)
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(length*8))
	padded = append(padded, lenBuf[:]...)

	state := sha256InitState(data[:7])
	for offset := 0; offset < len(padded); offset += 64 {
		words := make([]uint32, 16, 64)
		for i := 0; i < 16; i++ {
			words[i] = binary.BigEndian.Uint32(padded[offset+i*4 : offset+i*4+4])
		}
		expandSha256Schedule(&words, 16)
		working := sha256Rounds(state, words, 0)
		for i := 0; i < 8; i++ {
			state[i] += working[i]
		}
	}
	out := make([]byte, 32)
	for i := 0; i < 8; i++ {
		binary.BigEndian.PutUint32(out[i*4:i*4+4], state[i])
	}
	return out
}

func cs64Digest(data []byte, digest []byte) (uint32, uint32) {
	keyA := binary.LittleEndian.Uint32(digest[0:4]) | 1
	keyB := binary.LittleEndian.Uint32(digest[4:8]) | 1
	factors := [2]uint32{
		keyA * cs64Factor,
		keyB * cs64Factor,
	}
	terms := make([]uint32, 0, 14+len(data)/4+2)
	offsets := make([]uint32, 0, 14+len(data)/4+2)
	for idx, coef := range cs64Coefficients {
		terms = append(terms, coef[0]*factors[idx%2])
		offsets = append(offsets, coef[1])
	}
	zeroData := append([]byte{0x00}, data...)
	for idx, word := range wordsLE4Even(zeroData) {
		k := keyA
		if idx%2 != 0 {
			k = keyB
		}
		term := word * k
		m := uint32(0xEF0569FB)
		if idx%2 != 0 {
			m = uint32(0xC31713DB)
		}
		offset := rol16(word) * m
		if idx == 0 {
			term += cs64Adjustment0 * factors[0]
			offset += cs64Adjustment1
		}
		terms = append(terms, term)
		offsets = append(offsets, offset)
	}

	var swapAcc, swapLast, state, acc uint32
	for idx := 0; idx < len(terms); idx += 2 {
		x, y := terms[idx], terms[idx+1]
		t := rol16(offsets[idx] + x)
		q := t * 0x76F50F34
		swapAcc += rol16(q)*0xC3EFEA97 + q
		t = rol16(offsets[idx+1] + y)
		q = t * 0x1881CE3C
		swapLast = rol16(q)*0x35BD1EC9 + q
		swapAcc += swapLast

		state = state*keyA + x
		state = rol16(state) * 0xCF98B111
		state = rol16(state) * 0x87085B9F
		state = rol16(state) * 0x12CEB96D
		state = rol16(state) * 0x257E1D83
		acc += state
		t = state*keyB + y
		t = rol16(t) * 0xA27416F5
		t = rol16(t) * 0xD38396FF
		state = rol16(rol16(t)*0x7C932B89) * 0xBFA49F69
		acc += state
	}
	return swapLast ^ state, swapAcc ^ acc
}

var (
	tokenCacheMu sync.Mutex
	cachedSecGec string
	cachedSecExp time.Time
)

// GenerateSecMsGecToken generates the dynamic v2 Sec-MS-GEC anti-abuse token.
func GenerateSecMsGecToken(unixTimestamp float64) string {
	ticks := unixTimestamp + winEpoch
	rem := uint64(ticks) % 300
	ticks -= float64(rem)
	ticks *= 1e7 // 1e9 / 100

	strToHash := fmt.Sprintf("%.0f%s", ticks, trustedClientToken)
	digest := sha256Digest([]byte(strToHash))
	c1, c2 := cs64Digest([]byte(strToHash), digest)

	var cs64Buf [8]byte
	binary.LittleEndian.PutUint32(cs64Buf[0:4], c1)
	binary.LittleEndian.PutUint32(cs64Buf[4:8], c2)

	cs64Hex := strings.ToUpper(hex.EncodeToString(cs64Buf[:]))
	sha256Hex := strings.ToUpper(hex.EncodeToString(digest))
	return fmt.Sprintf("%s-%s", cs64Hex, sha256Hex)
}

// CurrentSecMsGecToken returns the cached or freshly generated v2 token.
func CurrentSecMsGecToken() string {
	tokenCacheMu.Lock()
	defer tokenCacheMu.Unlock()
	now := time.Now()
	if cachedSecGec != "" && now.Before(cachedSecExp) {
		return cachedSecGec
	}
	token := GenerateSecMsGecToken(float64(now.Unix()))
	cachedSecGec = token
	cachedSecExp = now.Add(2 * time.Minute)
	return token
}

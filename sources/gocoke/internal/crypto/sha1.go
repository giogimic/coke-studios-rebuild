package crypto

import (
	"fmt"
	"strings"
)

const shaHexChr = "0123456789abcdef"

// Sha1 replicates the C# custom SHA-1 implementation (including the ft() precedence quirk)
func Sha1(str string) string {
	x := shaStr2Blks(str)
	w := make([]int32, 80)
	a := int32(1732584193)
	b := int32(-271733879)
	c := int32(-1732584194)
	d := int32(271733878)
	e := int32(-1009589776)

	for i := 0; i < len(x); i += 16 {
		oldA := a
		oldB := b
		oldC := c
		oldD := d
		oldE := e

		for j := 0; j < 80; j++ {
			if j < 16 {
				w[j] = x[i+j]
			} else {
				w[j] = shaRol(w[j-3]^w[j-8]^w[j-14]^w[j-16], 1)
			}
			t := shaAdd(shaAdd(shaRol(a, 5), shaFt(j, b, c, d)), shaAdd(shaAdd(e, w[j]), shaKt(j)))
			e = d
			d = c
			c = shaRol(b, 30)
			b = a
			a = t
		}
		a = shaAdd(a, oldA)
		b = shaAdd(b, oldB)
		c = shaAdd(c, oldC)
		d = shaAdd(d, oldD)
		e = shaAdd(e, oldE)
	}
	return shaHex(a) + shaHex(b) + shaHex(c) + shaHex(d) + shaHex(e)
}

func shaHex(num int32) string {
	var sb strings.Builder
	for i := 7; i >= 0; i-- {
		val := (uint32(num) >> uint(i*4)) & 0x0f
		sb.WriteByte(shaHexChr[val])
	}
	return sb.String()
}

func shaStr2Blks(str string) []int32 {
	nblk := ((len(str) + 8) >> 6) + 1
	blks := make([]int32, nblk*16)

	for i := 0; i < len(str); i++ {
		blks[i>>2] |= int32(str[i]) << uint(24-(i%4)*8)
	}

	i := len(str)
	blks[i>>2] |= 0x80 << uint(24-(i%4)*8)
	blks[nblk*16-1] = int32(len(str) * 8)

	fmt.Println("Go blks:", blks)
	return blks
}

func shaAdd(x, y int32) int32 {
	lsw := (x & 0xFFFF) + (y & 0xFFFF)
	msw := (x >> 16) + (y >> 16) + (lsw >> 16)
	return (msw << 16) | (lsw & 0xFFFF)
}

func shaRol(num int32, cnt uint) int32 {
	return (num << cnt) | int32(uint32(num)>>(32-cnt))
}

// Important: this intentionally mirrors the C# precedence bug:
// C# had: (b & c) | (b ^ 0xFFFFFFFF & d)  ==> (b & c) | (b ^ d)
func shaFt(t int, b, c, d int32) int32 {
	if t < 20 {
		return (b & c) | (b ^ d)
	}
	if t < 40 {
		return b ^ c ^ d
	}
	if t < 60 {
		return (b & c) | (b & d) | (c & d)
	}
	return b ^ c ^ d
}

func shaKt(t int) int32 {
	if t < 20 {
		return 1518500249
	}
	if t < 40 {
		return 1859775393
	}
	if t < 60 {
		return -1894007588
	}
	return -899497514
}

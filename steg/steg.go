/*
Package steg provides steganographic encoding of messages
inside of PNG files.
*/
package steg

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

/*
Point represents a pixel coordinate in the image.
*/
type Point struct {
	X int
	Y int
}

func (p1 *Point) before(p2 Point) bool {
	if p1.Y >= p2.Y && p1.X >= p2.X {
		return false
	}
	return true
}

/*
Encoder has methods for writing and retrieving messages
written in PNG images. It defaults to encode messages to
the least significant bit.
*/
type Encoder struct {
	bit int
}

/*
SetMsgBit specifies which bit each byte will use for its
part of the message. If n is outside the range of 0-7
(inclusive) SetMsgBit will return an out of bounds error.
The least significant bit is zero and by default message
data will be written to this bit.
*/
func (e *Encoder) SetMsgBit(n int) error {
	if n < 0 || n > 7 {
		return errors.New("out of bounds")
	}
	e.bit = n
	return nil
}

/*
Encode takes the image at src and writes it to dst with msg
stored inside it. The value of start is a pixel coordinate
determining where the message will begin to be written.

Each byte of the image from start will contain one bit of msg
until msg is fully written. This means that msg needs len(msg)*8
bytes of space from start to store its entire payload.

Encode returns end which is the coordinates of the first pixel
after msg.

Encode will return an error if the start or the end points
of msg are outside the bounds of src.
*/
func (e *Encoder) Encode(src, dst, msg string, start Point) (end Point, err error) {

	src, err = filepath.Abs(src)
	if err != nil {
		return end, err
	}

	dst, err = filepath.Abs(dst)
	if err != nil {
		return end, err
	}

	r, err := os.Open(src)
	if err != nil {
		return end, err
	}
	defer r.Close()

	p, err := png.Decode(r)
	if err != nil {
		return end, err
	}

	img, ok := p.(*image.RGBA)
	if !ok {
		return end, errors.New("failed type assertion from image.Image to image.RGBA")
	}

	bounds := img.Bounds()
	end = pointAtOffset(bounds, start, len(msg)*8)

	if !inBounds(bounds, start) {
		return end, errors.New("start point out of bounds")
	}
	if !inBounds(bounds, end) {
		return end, errors.New("end point out of bounds")
	}

	tmp := make([]byte, 8)

	var i uint
	offset := offsetFromMin(bounds, start)

outer:
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {

		for x := bounds.Min.X; x < bounds.Max.X; x++ {

			if x < start.X || y < start.Y {
				i++
				continue
			}

			if x == end.X && y == end.Y {
				break outer
			}

			mod := i % 8

			if mod == 0 {
				byteToBits(tmp, msg[(i-offset)/8])
			}

			r, g, b, a := img.At(x, y).RGBA()

			if tmp[mod] == 1 { // set bit
				r |= uint32(pow(2, e.bit))
			} else { // clear bit
				r = uint32(byte(r) & ^(byte(pow(2, e.bit))))
			}

			img.Set(x, y, color.RGBA{
				uint8(r),
				uint8(g),
				uint8(b),
				uint8(a),
			})

			i++
		}
	}

	w, err := os.Create(dst)
	if err != nil {
		return end, err
	}
	defer w.Close()

	err = png.Encode(w, p)
	if err != nil {
		return end, err
	}

	return end, nil
}

/*
Decode reads src from start to end and extracts msg.

Returns an error if start or end are outside the
boundaries of src or if start does not precede end.
*/
func (e *Encoder) Decode(src string, start, end Point) (msg string, err error) {

	src, err = filepath.Abs(src)
	if err != nil {
		return msg, err
	}

	r, err := os.Open(src)
	if err != nil {
		return msg, err
	}
	defer r.Close()

	p, err := png.Decode(r)
	if err != nil {
		return msg, err
	}

	img, ok := p.(*image.RGBA)
	if !ok {
		return msg, errors.New("failed type assertion from image.Image to image.RGBA")
	}

	bounds := img.Bounds()

	if !start.before(end) {
		return msg, errors.New("start point does not precede end point")
	}
	if !inBounds(bounds, start) {
		return msg, errors.New("start point out of bounds")
	}
	if !inBounds(bounds, end) {
		return msg, errors.New("end point out of bounds")
	}

	tmp := make([]byte, 8)

	var i uint

outer:
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {

		for x := bounds.Min.X; x < bounds.Max.X; x++ {

			if x < start.X || y < start.Y {
				i++
				continue
			}

			if x == end.X && y == end.Y {
				break outer
			}

			mod := i % 8

			r, _, _, _ := img.At(x, y).RGBA()

			if byte(r)&byte(pow(2, e.bit)) == 0 {
				tmp[mod] = 0
			} else {
				tmp[mod] = 1
			}

			if mod == 8-1 {
				n, _ := bitsToByte(tmp)
				msg += string(n)
			}

			i++
		}
	}

	return msg, nil
}

func inBounds(r image.Rectangle, p Point) bool {
	if p.X < r.Min.X {
		return false
	}
	if p.Y < r.Min.Y {
		return false
	}
	if p.X >= r.Max.X {
		return false
	}
	if p.Y >= r.Max.Y {
		return false
	}
	return true
}

func offsetFromMin(r image.Rectangle, p Point) uint {
	width := uint(r.Max.X) - uint(r.Min.X)
	return (uint(p.Y) * uint(width)) + uint(p.X)
}

func pointAtOffset(r image.Rectangle, p Point, offset int) Point {

	width := r.Max.X - r.Min.X
	rows := offset / width
	p.Y += rows

	offset -= rows * width

	if p.X+offset > r.Max.X {
		p.Y++
		offset -= r.Max.X - p.X
		p.X = r.Min.X
	}

	p.X += offset

	return p
}

func bitsToByte(bits []byte) (b byte, err error) {

	if len(bits) != 8 {
		return b, errors.New("byte slice must be exactly len(8)")
	}

	for i, bit := range bits {

		// Bit position; e.g. 128, 64, 32, 16, etc
		pos := pow(2, 7-i)

		if bit == 1 {
			b += byte(pos)
		}
	}

	return b, nil
}

func byteToBits(bits []byte, b byte) error {

	if len(bits) != 8 {
		return errors.New("byte slice must be exactly len(8)")
	}

	for i := range bits {

		// Bit position; e.g. 128, 64, 32, 16, etc
		pos := pow(2, 7-i)

		if int(b)-pos >= 0 {
			bits[i] = 1
			b -= byte(pos)
		} else {
			bits[i] = 0
		}
	}

	return nil
}

func pow(x, y int) int {
	return int(math.Pow(float64(x), float64(y)))
	// n := x
	// if y == 0 {
	// 	return 1
	// }
	// for i := 0; i < y-1; i++ {
	// 	x *= n
	// }
	// return x
}

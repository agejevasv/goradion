package radio

import (
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// The QR spec asks for a quiet zone of 4 modules; 2 keeps the code within an
// 80x24 terminal and phone scanners cope with it.
const qrQuietZone = 2

// qrText renders a QR code as half-block characters, two modules per text row,
// in explicit black-on-white so it scans on any terminal colour scheme. Width
// and height are in terminal cells.
func qrText(content string) (text string, width, height int, err error) {
	qr, err := qrcode.New(content, qrcode.Low)
	if err != nil {
		return "", 0, 0, err
	}
	qr.DisableBorder = true

	bits := qr.Bitmap()
	size := len(bits)
	width = size + 2*qrQuietZone

	dark := func(x, y int) bool {
		x -= qrQuietZone
		y -= qrQuietZone
		return x >= 0 && y >= 0 && x < size && y < size && bits[y][x]
	}

	var sb strings.Builder
	for y := 0; y < width; y += 2 {
		sb.WriteString("[black:white]")
		for x := 0; x < width; x++ {
			top, bottom := dark(x, y), dark(x, y+1)
			switch {
			case top && bottom:
				sb.WriteRune('█')
			case top:
				sb.WriteRune('▀')
			case bottom:
				sb.WriteRune('▄')
			default:
				sb.WriteRune(' ')
			}
		}
		sb.WriteString("[-:-]\n")
		height++
	}

	return sb.String(), width, height, nil
}

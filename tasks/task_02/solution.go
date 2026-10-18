package main

func rotateRunes(s string, shift int) string {
	runes := []rune(s)
	length := len(runes)

	if length == 0 {
		return s
	}
	shift = (shift%length + length) % length
	if shift == 0 {
		return string(runes)
	}

	rotated := append(runes[shift:], runes[:shift]...)
	return string(rotated)
}

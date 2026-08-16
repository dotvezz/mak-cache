package cache

// approximateStringHeapSize does its thing with the assumption that the compilation target is
func approximateStringHeapSize64(size int) int {
	switch {
	case size == 0:
		return 0
	case size <= 8:
		return 8
	case size <= 16:
		return 16
	case size <= 24:
		return 24
	case size <= 32:
		return 32
	case size <= 48:
		return 48
	case size <= 64:
		return 64
	case size <= 80:
		return 80
	case size <= 96:
		return 96
	case size <= 112:
		return 112
	case size <= 128:
		return 128
	case size <= 144:
		return 144
	case size <= 160:
		return 160
	case size <= 176:
		return 176
	case size <= 192:
		return 192
	case size <= 208:
		return 208
	case size <= 224:
		return 224
	case size <= 240:
		return 240
	case size <= 256:
		return 256
	case size <= 320:
		return 320
	case size <= 384:
		return 384
	case size <= 448:
		return 448
	case size <= 512:
		return 512
	default:
		return (size + 63) &^ 63 // align to 64 bytes for larger entries
	}
}

package qris

import (
	"fmt"
	"strings"
)

// convertCRC16 computes the CRC-16/CCITT-FALSE checksum used by QRIS.
func convertCRC16(str string) string {
	crc := uint32(0xFFFF)
	for _, ch := range []byte(str) {
		crc ^= uint32(ch) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	hex := crc & 0xFFFF
	return strings.ToUpper(fmt.Sprintf("%04X", hex))
}

// ConvertToDynamic converts a static QRIS string to a dynamic QRIS string
// by injecting the given amount (in IDR, e.g. "15000") into the payload.
//
// Steps (per QRIS spec):
//  1. Strip the last 4 characters (existing CRC).
//  2. Flip the presentation indicator from "0102 11" (static) to "0102 12" (dynamic).
//  3. Inject the transaction amount field (tag 54) before the country code (5802ID).
//  4. Recompute and append the CRC-16 checksum.
func ConvertToDynamic(staticQRIS string, amountIDR string) string {
	// 1. Remove last 4 chars (CRC)
	withoutCRC := staticQRIS[:len(staticQRIS)-4]

	// 2. Change presentation mode: static (010211) → dynamic (010212)
	step1 := strings.ReplaceAll(withoutCRC, "010211", "010212")

	// 3. Split on the country code tag and inject amount field
	parts := strings.SplitN(step1, "5802ID", 2)
	if len(parts) != 2 {
		// If the QRIS string is malformed, return empty string
		return ""
	}

	// Tag 54 = Transaction Amount
	// Format: "54" + 2-digit length + amount
	amountField := fmt.Sprintf("54%02d%s5802ID", len(amountIDR), amountIDR)

	combined := strings.TrimSpace(parts[0]) + amountField + strings.TrimSpace(parts[1])

	// 4. Recompute CRC and append
	return combined + convertCRC16(combined)
}

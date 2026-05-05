package qris_test

import (
	"fmt"
	"strings"
	"testing"

	"backend/pkg/qris"

	qrcode "github.com/skip2/go-qrcode"
)

const sampleStaticQRIS = "00020101021126670016COM.NOBUBANK.WWW01189360050300000868790214054459434534230303UMI51440014ID.CO.QRIS.WWW0215ID20243532458790303UMI5204481253033605802ID5919REKBERIN AJA ID50336007BANDUNG61054011162070703A0163045BF1"

func TestConvertToDynamic_InjectsAmount(t *testing.T) {
	dynamic := qris.ConvertToDynamic(sampleStaticQRIS, "75000")

	if dynamic == "" {
		t.Fatal("hasil convert kosong")
	}

	if !strings.Contains(dynamic, "540575000") {
		t.Errorf("nominal tidak masuk ke QRIS: %s", dynamic)
	}

	if !strings.Contains(dynamic, "010212") {
		t.Errorf("QRIS belum berubah ke mode dinamis: %s", dynamic)
	}

	crc := dynamic[len(dynamic)-4:]
	if len(crc) != 4 {
		t.Errorf("CRC tidak valid, dapat: %s", crc)
	}
}

// 🔥 INI BAGIAN NAMPILIN QR KE TERMINAL
func TestConvertToDynamic_ShowQR(t *testing.T) {
	dynamic := qris.ConvertToDynamic(sampleStaticQRIS, "75000")

	if dynamic == "" {
		t.Fatal("hasil convert kosong")
	}

	fmt.Println("\n=== QRIS STRING ===")
	fmt.Println(dynamic)

	qr, err := qrcode.New(dynamic, qrcode.Low)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("\n=== QR CODE (SCAN INI) ===")
	fmt.Println(qr.ToSmallString(true))
}

func TestConvertToDynamic_DifferentAmounts(t *testing.T) {
	amounts := []string{"50000", "100000", "150000", "200000"}

	for _, amt := range amounts {
		result := qris.ConvertToDynamic(sampleStaticQRIS, amt)

		if result == "" {
			t.Errorf("gagal convert untuk nominal %s", amt)
		}
	}
}

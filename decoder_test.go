package tta

import (
	"os"
	"testing"
)

func TestDecompress(t *testing.T) {
	t.Parallel()
	infile, err := os.Open("./data/sample.tta")
	if err != nil {
		t.Fatal(err)
	}
	defer infile.Close()
	outfile, err := os.Create(os.TempDir() + "/sample_decompressed.wav")
	if err != nil {
		t.Fatal(err)
	}
	defer outfile.Close()
	defer os.Remove(os.TempDir() + "/sample_decompressed.wav")
	if err = Decompress(infile, outfile, "", nil); err != nil {
		t.Error(err)
	}
}

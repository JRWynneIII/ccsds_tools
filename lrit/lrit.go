package lrit

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

type File struct {
	Version                   uint8
	VCDUVersion               uint8
	PrimaryHeader             PrimaryHeader
	SecondaryHeaders          []SecondaryHeader
	Data                      []byte
	CRCGood                   bool
	RawData                   []byte
	VCID                      uint8
	UnzippedData              map[string][]byte
	RawHeaders                []byte
	SecondaryHeadersPopulated bool
	PrimaryHeaderPopulated    bool
}

var (
	LRITCRCMismatchErr    error = fmt.Errorf("LRIT file CRC mismatch")
	LRITLengthMismatchErr error = fmt.Errorf("LRIT file length mismatch")
	LRITPrimaryHeaderErr  error = fmt.Errorf("Invalid LRIT Primary header")
)

func NewExistingFile(path string) (*File, error) {
	if data, err := os.ReadFile(path); err == nil {
		ph, _ := MakePrimaryHeader(data)
		headers := data[:16]
		data = data[16:]
		lf := File{
			PrimaryHeader: ph,
			Data:          data,
			CRCGood:       true,
			RawHeaders:    headers,
		}

		if err = lf.PopulateSecondaryHeaders(); err != nil {
			return nil, fmt.Errorf("Could not parse LRIT file headers (%s): %s", path, err.Error())
		}
		return &lf, nil
	} else {
		return nil, fmt.Errorf("Can not read LRIT file %s", path)
	}
}

func (l File) IsImageFile() bool {
	hasImgStructHeader := slices.ContainsFunc(l.SecondaryHeaders, func(a SecondaryHeader) bool {
		return a.HeaderType() == ImageStructureHeaderType
	})

	if l.PrimaryHeader.FileType == 0 && hasImgStructHeader {
		return true
	}

	return false
}

func (l File) GetName() string {
	for _, sh := range l.SecondaryHeaders {
		if sh.HeaderType() == AnnotationHeaderType && strings.Contains(sh.(AnnotationHeader).Text, ".lrit") {
			return sh.(AnnotationHeader).Text
		}
	}
	return ""
}

func (f File) IsValid() (bool, error) {
	if !f.PrimaryHeader.IsValid() {
		return false, LRITPrimaryHeaderErr
	}
	if uint64(len(f.Data)) != f.PrimaryHeader.DataLength/8 {
		return false, LRITLengthMismatchErr
	}
	if !f.CRCGood {
		return false, LRITCRCMismatchErr
	}
	return true, nil
}

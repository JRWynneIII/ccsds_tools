package lrit

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/opensatelliteproject/goaec/szwrap"
)

type File struct {
	Version          uint8
	VCDUVersion      uint8
	PrimaryHeader    PrimaryHeader
	SecondaryHeaders []SecondaryHeader
	Data             []byte
	//HaveCRC          uint16
	//WantCRC          uint16
	CRCGood bool
	RawData []byte
	VCID    uint8
}

type PrimaryHeader struct {
	Type            uint8
	Length          uint16
	FileType        uint8
	AllHeaderLength uint32
	DataLength      uint64
}

type SecondaryHeaderType uint8

const (
	ImageStructureHeaderType        SecondaryHeaderType = 1
	ImageNavigationHeaderType                           = 2
	ImageDataFunctionHeaderType                         = 3
	AnnotationHeaderType                                = 4
	TimestampHeaderType                                 = 5
	AncillaryTextHeaderType                             = 6
	KeyHeaderType                                       = 7
	SegmentIdentificationHeaderType                     = 128
	NOAASpecificHeaderType                              = 129
	HeaderStructureRecordHeaderType                     = 130
	RiceCompressionHeaderType                           = 131
)

type SecondaryHeader interface {
	HeaderType() SecondaryHeaderType
	HeaderLength() uint16
}

type ImageStructureHeader struct {
	Type         uint8
	Length       uint16
	BitsPerPixel uint8
	NumCols      uint16
	NumRows      uint16
	IsCompressed uint8
}

type ImageNavigationHeader struct {
	Type                uint8
	Length              uint16
	ProjectionName      string //32 bytes
	ColumnScalingFactor uint32
	LineScalingFactor   uint32
	ColumnOffset        uint32
	LineOffset          uint32
}

type ImageDataFunctionHeader struct {
	Type           uint8
	Length         uint16
	DataDefinition string
}

type AnnotationHeader struct {
	Type   uint8
	Length uint16
	Text   string
}

type TimestampHeader struct {
	Type   uint8
	Length uint16
	Time   uint64 //TODO: Maybe change this to a time.Time
}

type AncillaryTextHeader struct {
	Type   uint8
	Length uint16
	Text   string
}

type KeyHeader struct {
	Type   uint8 //Used to control compression. Ignore if Type == 7
	Length uint16
}

type SegmentIdentificationHeader struct {
	Type            uint8
	Length          uint16
	ImageIdentifier uint16
	SequenceNumber  uint16
	StartColumn     uint16
	StartLine       uint16
	MaxSegment      uint16
	MaxColumn       uint16
	MaxRow          uint16
}

type NOAASpecificHeader struct {
	Type                    uint8
	Length                  uint16
	Agency                  string
	ProductID               uint16
	ProductSubID            uint16
	Parameter               uint16
	NOAASpecificCompression uint8
}

type HeaderStructureRecordHeader struct {
	Type      uint8
	Length    uint16
	Structure string
}

type RiceCompressionHeader struct {
	Type               uint8
	Length             uint16
	Flags              uint16
	PixelsPerBlock     uint8
	ScanlinesPerPacket uint8
}

func NewExistingFile(path string) (*File, error) {
	if data, err := os.ReadFile(path); err == nil {
		ph := MakePrimaryHeader(data)
		data = data[16:]
		lf := File{
			PrimaryHeader: ph,
			Data:          data,
			CRCGood:       true,
		}

		if err = lf.PopulateSecondaryHeaders(); err != nil {
			return nil, fmt.Errorf("Could not parse LRIT file headers (%s): %s", path, err.Error())
		}
		return &lf, nil
	} else {
		return nil, fmt.Errorf("Can not read LRIT file %s", path)
	}
}

var filecounter map[string]int = make(map[string]int)

func (l File) WriteFile(dir string) {
	path := filepath.Join(dir, l.GetName())
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		path = filepath.Join(dir, fmt.Sprintf("%s_%d_%d", l.GetName(), l.VCDUVersion, filecounter[l.GetName()]))
		filecounter[l.GetName()] += 1
	}
	if err := os.WriteFile(path, l.RawData, os.FileMode(0644)); err != nil {
		log.Errorf("Could not write file %s", path)
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

func (l File) ContainsZipArchive() bool {
	nsh := l.FindSecondaryHeader(NOAASpecificHeaderType).(NOAASpecificHeader)
	if l.PrimaryHeader.FileType != 0 && nsh.NOAASpecificCompression > 1 {
		return true
	}
	return false
}

func (l File) UnzipToBuffer() (map[string][]byte, error) {
	ret := make(map[string][]byte)
	if zr, err := zip.NewReader(bytes.NewReader(l.Data), int64(len(l.Data))); err == nil {
		for _, file := range zr.File {
			if f, err := file.Open(); err == nil {
				defer f.Close()
				if ret[file.Name], err = io.ReadAll(f); err != nil {
					return ret, err
				}
			} else {
				return ret, err
			}
		}
		return ret, nil
	} else {
		return ret, err
	}
}

func (l File) GetName() string {
	for _, sh := range l.SecondaryHeaders {
		if sh.HeaderType() == AnnotationHeaderType && strings.Contains(sh.(AnnotationHeader).Text, ".lrit") {
			return sh.(AnnotationHeader).Text
		}
	}
	return ""
}

func (l File) FindSecondaryHeader(htype SecondaryHeaderType) SecondaryHeader {
	for _, sh := range l.SecondaryHeaders {
		if sh.HeaderType() == htype {
			return sh
		}
	}
	return nil
}

func (l File) IsRiceCompressed() bool {
	if sh := l.FindSecondaryHeader(ImageStructureHeaderType); sh != nil {
		if sh.(ImageStructureHeader).IsCompressed == 1 {
			return true
		}
	}
	return false
}

func RiceDecompressBuffer(data []byte, rch RiceCompressionHeader, ish ImageStructureHeader) ([]byte, error) {
	pixels := rch.PixelsPerBlock
	flags := rch.Flags
	cols := ish.NumCols
	var ret []byte
	if decompresseddata, err := szwrap.NOAADecompress(data, int(ish.BitsPerPixel), int(pixels), int(cols), int(flags)); err == nil {
		ret = decompresseddata
	} else {
		return data, err
	}

	return ret, nil
}

func (l *File) RiceDecompress() error {
	rch := l.FindSecondaryHeader(RiceCompressionHeaderType).(RiceCompressionHeader)
	ish := l.FindSecondaryHeader(ImageStructureHeaderType).(ImageStructureHeader)
	pixels := rch.PixelsPerBlock
	flags := rch.Flags
	cols := ish.NumCols
	if data, err := szwrap.NOAADecompress(l.Data, int(ish.BitsPerPixel), int(pixels), int(cols), int(flags)); err == nil {
		l.Data = data
	} else {
		return err
	}

	return nil
}

func MakeImageStructureHeader(ph PrimaryHeader, data []byte) ImageStructureHeader {
	//nextHeaderType := data[0]
	//curidx := 0
	//for uint32(curidx) < ph.AllHeaderLength {
	//	if SecondaryHeaderType(nextHeaderType) == ImageStructureHeaderType {
	//		break
	//	}
	//	headerlen := (uint16(data[curidx+1]) << 8) | uint16(data[curidx+2])
	//	curidx += int(headerlen)
	//}
	curhtype := data[0]
	for SecondaryHeaderType(curhtype) != ImageStructureHeaderType && len(data) >= 3 {
		headerlen := (uint16(data[1]) << 8) | uint16(data[2])
		if headerlen > uint16(len(data)) {
			log.Errorf("Could not find Image Structure Header")
			return ImageStructureHeader{}
		}
		data = data[headerlen:]
	}

	if len(data) <= 3 {
		return ImageStructureHeader{}
	}

	htype := data[0]
	headerlen := (uint16(data[1]) << 8) | uint16(data[2])
	return ImageStructureHeader{
		Type:         htype,
		Length:       headerlen,
		BitsPerPixel: data[3],
		NumCols:      (uint16(data[4]) << 8) | uint16(data[5]),
		NumRows:      (uint16(data[6]) << 8) | uint16(data[7]),
		IsCompressed: data[8],
	}
}

func MakeRiceCompressionHeader(ph PrimaryHeader, data []byte) RiceCompressionHeader {
	//nextHeaderType := data[0]
	//curidx := 0
	//for uint32(curidx) < ph.AllHeaderLength {
	//	if SecondaryHeaderType(nextHeaderType) == RiceCompressionHeaderType {
	//		break
	//	}
	//	headerlen := (uint16(data[curidx+1]) << 8) | uint16(data[curidx+2])
	//	curidx += int(headerlen)
	//}

	//htype := data[curidx]
	//headerlen := (uint16(data[curidx+1]) << 8) | uint16(data[curidx+2])
	curhtype := data[0]
	for SecondaryHeaderType(curhtype) != RiceCompressionHeaderType && len(data) >= 3 {
		headerlen := (uint16(data[1]) << 8) | uint16(data[2])
		if headerlen > uint16(len(data)) {
			log.Errorf("Could not find Rice Compression Header")
			return RiceCompressionHeader{}
		}
		data = data[headerlen:]
	}

	if len(data) <= 3 {
		return RiceCompressionHeader{}
	}

	htype := data[0]
	headerlen := (uint16(data[1]) << 8) | uint16(data[2])
	return RiceCompressionHeader{
		Type:               htype,
		Length:             headerlen,
		Flags:              (uint16(data[3]) << 8) | uint16(data[4]),
		PixelsPerBlock:     data[5],
		ScanlinesPerPacket: data[6],
	}
}

func (l File) getNextHeader() (SecondaryHeader, error) {
	htype := l.Data[0]
	headerlen := (uint16(l.Data[1]) << 8) | uint16(l.Data[2])
	switch SecondaryHeaderType(htype) {
	case ImageStructureHeaderType:
		return ImageStructureHeader{
			Type:         htype,
			Length:       headerlen,
			BitsPerPixel: l.Data[3],
			NumCols:      (uint16(l.Data[4]) << 8) | uint16(l.Data[5]),
			NumRows:      (uint16(l.Data[6]) << 8) | uint16(l.Data[7]),
			IsCompressed: l.Data[8],
		}, nil
	case ImageNavigationHeaderType:
		return ImageNavigationHeader{
			Type:                htype,
			Length:              headerlen,
			ProjectionName:      string(l.Data[3:35]),
			ColumnScalingFactor: (uint32(l.Data[35]) << 24) | (uint32(l.Data[36]) << 16) | (uint32(l.Data[37]) << 8) | uint32(l.Data[38]),
			LineScalingFactor:   (uint32(l.Data[39]) << 24) | (uint32(l.Data[40]) << 16) | (uint32(l.Data[41]) << 8) | uint32(l.Data[42]),
			ColumnOffset:        (uint32(l.Data[43]) << 24) | (uint32(l.Data[44]) << 16) | (uint32(l.Data[45]) << 8) | uint32(l.Data[46]),
			LineOffset:          (uint32(l.Data[47]) << 24) | (uint32(l.Data[48]) << 16) | (uint32(l.Data[49]) << 8) | uint32(l.Data[50]),
		}, nil
	case ImageDataFunctionHeaderType:
		headerlen := (uint16(l.Data[1]) << 8) | uint16(l.Data[2])
		return ImageDataFunctionHeader{
			Type:           htype,
			Length:         headerlen,
			DataDefinition: string(l.Data[3:headerlen]), // TODO: might need to be either -2 or -4?
		}, nil
	case AnnotationHeaderType:
		headerlen := (uint16(l.Data[1]) << 8) | uint16(l.Data[2])
		return AnnotationHeader{
			Type:   htype,
			Length: headerlen,
			Text:   string(l.Data[3:headerlen]),
		}, nil
	case TimestampHeaderType:
		return TimestampHeader{
			Type:   htype,
			Length: headerlen,
			Time:   (uint64(l.Data[3]) << 48) | (uint64(l.Data[4]) << 40) | (uint64(l.Data[5]) << 32) | (uint64(l.Data[6]) << 24) | (uint64(l.Data[7]) << 16) | (uint64(l.Data[8]) << 8) | uint64(l.Data[9]),
		}, nil
	case AncillaryTextHeaderType:
		headerlen := (uint16(l.Data[1]) << 8) | uint16(l.Data[2])
		return AncillaryTextHeader{
			Type:   htype,
			Length: headerlen,
			Text:   string(l.Data[3:headerlen]),
		}, nil
	case KeyHeaderType:
		return KeyHeader{
			Type:   htype,
			Length: headerlen,
		}, nil
	case SegmentIdentificationHeaderType:
		return SegmentIdentificationHeader{
			Type:            htype,
			Length:          headerlen,
			ImageIdentifier: (uint16(l.Data[3]) << 8) | uint16(l.Data[4]),
			SequenceNumber:  (uint16(l.Data[5]) << 8) | uint16(l.Data[6]),
			StartColumn:     (uint16(l.Data[7]) << 8) | uint16(l.Data[8]),
			StartLine:       (uint16(l.Data[9]) << 8) | uint16(l.Data[10]),
			MaxSegment:      (uint16(l.Data[11]) << 8) | uint16(l.Data[12]),
			MaxColumn:       (uint16(l.Data[13]) << 8) | uint16(l.Data[14]),
			MaxRow:          (uint16(l.Data[15]) << 8) | uint16(l.Data[16]),
		}, nil
	case NOAASpecificHeaderType:
		return NOAASpecificHeader{
			Type:                    htype,
			Length:                  headerlen,
			Agency:                  string(l.Data[3:7]),
			ProductID:               (uint16(l.Data[7]) << 8) | uint16(l.Data[8]),
			ProductSubID:            (uint16(l.Data[9]) << 8) | uint16(l.Data[10]),
			Parameter:               (uint16(l.Data[11]) << 8) | uint16(l.Data[12]),
			NOAASpecificCompression: l.Data[13],
		}, nil
	case HeaderStructureRecordHeaderType:
		return HeaderStructureRecordHeader{
			Type:      htype,
			Length:    headerlen,
			Structure: string(l.Data[3:headerlen]),
		}, nil
	case RiceCompressionHeaderType:
		return RiceCompressionHeader{
			Type:               htype,
			Length:             headerlen,
			Flags:              (uint16(l.Data[3]) << 8) | uint16(l.Data[4]),
			PixelsPerBlock:     l.Data[5],
			ScanlinesPerPacket: l.Data[6],
		}, nil
	default:
		return nil, fmt.Errorf("Invalid file type found in header! (type = %d)", l.PrimaryHeader.FileType)
	}
}

func (l *File) PopulateSecondaryHeaders() error {
	headerlen := int(l.PrimaryHeader.AllHeaderLength) - int(l.PrimaryHeader.Length)
	for headerlen > 0 {
		if sh, err := l.getNextHeader(); err == nil {
			l.SecondaryHeaders = append(l.SecondaryHeaders, sh)
			headerlen -= int(sh.HeaderLength())
			l.Data = l.Data[sh.HeaderLength():]
		} else {
			return err
		}
	}
	return nil
}

func (h PrimaryHeader) IsValid() bool {
	if h.Type != 0 {
		return false
	}
	return true
}

var LRITCRCMismatchErr error = fmt.Errorf("LRIT file CRC mismatch")
var LRITLengthMismatchErr error = fmt.Errorf("LRIT file length mismatch")
var LRITPrimaryHeaderErr error = fmt.Errorf("Invalid LRIT Primary header")

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

func MakePrimaryHeader(data []byte) PrimaryHeader {
	return PrimaryHeader{
		Type:            data[0],
		Length:          (uint16(data[1]) << 8) | uint16(data[2]),
		FileType:        data[3],
		AllHeaderLength: (uint32(data[4]) << 24) | (uint32(data[5]) << 16) | (uint32(data[6]) << 8) | uint32(data[7]),
		DataLength:      (uint64(data[8]) << 56) | (uint64(data[9]) << 48) | (uint64(data[10]) << 40) | (uint64(data[11]) << 32) | (uint64(data[12]) << 24) | (uint64(data[13]) << 16) | (uint64(data[14]) << 8) | uint64(data[15]),
	}
}

func (h ImageStructureHeader) HeaderType() SecondaryHeaderType {
	return ImageStructureHeaderType
}

func (h ImageNavigationHeader) HeaderType() SecondaryHeaderType {
	return ImageNavigationHeaderType
}

func (h ImageDataFunctionHeader) HeaderType() SecondaryHeaderType {
	return ImageDataFunctionHeaderType
}

func (h AnnotationHeader) HeaderType() SecondaryHeaderType {
	return AnnotationHeaderType
}

func (h TimestampHeader) HeaderType() SecondaryHeaderType {
	return TimestampHeaderType
}

func (h AncillaryTextHeader) HeaderType() SecondaryHeaderType {
	return AncillaryTextHeaderType
}

func (h KeyHeader) HeaderType() SecondaryHeaderType {
	return KeyHeaderType
}

func (h SegmentIdentificationHeader) HeaderType() SecondaryHeaderType {
	return SegmentIdentificationHeaderType
}

func (h NOAASpecificHeader) HeaderType() SecondaryHeaderType {
	return NOAASpecificHeaderType
}

func (h HeaderStructureRecordHeader) HeaderType() SecondaryHeaderType {
	return HeaderStructureRecordHeaderType
}

func (h RiceCompressionHeader) HeaderType() SecondaryHeaderType {
	return RiceCompressionHeaderType
}

func (h ImageStructureHeader) HeaderLength() uint16 {
	return h.Length
}

func (h ImageNavigationHeader) HeaderLength() uint16 {
	return h.Length
}

func (h ImageDataFunctionHeader) HeaderLength() uint16 {
	return h.Length
}

func (h AnnotationHeader) HeaderLength() uint16 {
	return h.Length
}

func (h TimestampHeader) HeaderLength() uint16 {
	return h.Length
}

func (h AncillaryTextHeader) HeaderLength() uint16 {
	return h.Length
}

func (h KeyHeader) HeaderLength() uint16 {
	return h.Length
}

func (h SegmentIdentificationHeader) HeaderLength() uint16 {
	return h.Length
}

func (h NOAASpecificHeader) HeaderLength() uint16 {
	return h.Length
}

func (h HeaderStructureRecordHeader) HeaderLength() uint16 {
	return h.Length
}

func (h RiceCompressionHeader) HeaderLength() uint16 {
	return h.Length
}

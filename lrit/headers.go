package lrit

import (
	"fmt"

	"github.com/charmbracelet/log"
)

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
	DCSFilenameHeaderType                               = 132
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
	ProjectionName      string
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

type DCSFilenameHeader struct {
	Type     uint8
	Length   uint16
	Filename string
}

func (f *File) GetImageStructureHeader() (ImageStructureHeader, error) {
	tmpish := f.FindSecondaryHeader(ImageStructureHeaderType)
	if tmpish != nil {
		return tmpish.(ImageStructureHeader), nil
	}
	return ImageStructureHeader{}, fmt.Errorf("Could not find Image Structure Header")
}

func (f *File) GetRiceCompressionHeader() (RiceCompressionHeader, error) {
	tmprch := f.FindSecondaryHeader(RiceCompressionHeaderType)
	if tmprch != nil {
		return tmprch.(RiceCompressionHeader), nil
	}
	return RiceCompressionHeader{}, fmt.Errorf("Could not find Rice Compression Header")
}

func (f *File) GetNOAASpecificHeader() (NOAASpecificHeader, error) {
	tmpnsh := f.FindSecondaryHeader(NOAASpecificHeaderType)
	if tmpnsh != nil {
		return tmpnsh.(NOAASpecificHeader), nil
	}
	return NOAASpecificHeader{}, fmt.Errorf("Could not find Rice Compression Header")
}

func (l File) FindSecondaryHeader(htype SecondaryHeaderType) SecondaryHeader {
	for _, sh := range l.SecondaryHeaders {
		if sh.HeaderType() == htype {
			return sh
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

func (l *File) PopulateSecondaryHeaders() error {
	if uint32(len(l.Data))-l.PrimaryHeader.AllHeaderLength <= uint32(0) {
		return fmt.Errorf("Not enough data to process all headers!")
	}
	headerlen := int(l.PrimaryHeader.AllHeaderLength) - int(l.PrimaryHeader.Length)
	for headerlen > 0 {
		if sh, raw, err := getNextHeader(l.Data); err == nil {
			l.SecondaryHeaders = append(l.SecondaryHeaders, sh)
			l.RawHeaders = append(l.RawHeaders, raw...)
			headerlen -= int(sh.HeaderLength())
			l.Data = l.Data[sh.HeaderLength():]
		} else {
			return err
		}
	}
	return nil
}

func MakeImageStructureHeader(ph PrimaryHeader, data []byte) ImageStructureHeader {
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

func getNextHeader(data []byte) (SecondaryHeader, []byte, error) {
	if len(data) < 3 {
		return nil, []byte{}, fmt.Errorf("Packet too short! Could not make secondary header...")
	}

	htype := data[0]
	headerlen := (uint16(data[1]) << 8) | uint16(data[2])

	switch SecondaryHeaderType(htype) {
	case ImageStructureHeaderType:
		return ImageStructureHeader{
			Type:         htype,
			Length:       headerlen,
			BitsPerPixel: data[3],
			NumCols:      (uint16(data[4]) << 8) | uint16(data[5]),
			NumRows:      (uint16(data[6]) << 8) | uint16(data[7]),
			IsCompressed: data[8],
		}, data[:headerlen], nil
	case ImageNavigationHeaderType:
		return ImageNavigationHeader{
			Type:                htype,
			Length:              headerlen,
			ProjectionName:      string(data[3:35]),
			ColumnScalingFactor: (uint32(data[35]) << 24) | (uint32(data[36]) << 16) | (uint32(data[37]) << 8) | uint32(data[38]),
			LineScalingFactor:   (uint32(data[39]) << 24) | (uint32(data[40]) << 16) | (uint32(data[41]) << 8) | uint32(data[42]),
			ColumnOffset:        (uint32(data[43]) << 24) | (uint32(data[44]) << 16) | (uint32(data[45]) << 8) | uint32(data[46]),
			LineOffset:          (uint32(data[47]) << 24) | (uint32(data[48]) << 16) | (uint32(data[49]) << 8) | uint32(data[50]),
		}, data[:headerlen], nil
	case ImageDataFunctionHeaderType:
		headerlen := (uint16(data[1]) << 8) | uint16(data[2])
		return ImageDataFunctionHeader{
			Type:           htype,
			Length:         headerlen,
			DataDefinition: string(data[3:headerlen]), // TODO: might need to be either -2 or -4?
		}, data[:headerlen], nil
	case AnnotationHeaderType:
		headerlen := (uint16(data[1]) << 8) | uint16(data[2])
		return AnnotationHeader{
			Type:   htype,
			Length: headerlen,
			Text:   string(data[3:headerlen]),
		}, data[:headerlen], nil
	case TimestampHeaderType:
		return TimestampHeader{
			Type:   htype,
			Length: headerlen,
			Time:   (uint64(data[3]) << 48) | (uint64(data[4]) << 40) | (uint64(data[5]) << 32) | (uint64(data[6]) << 24) | (uint64(data[7]) << 16) | (uint64(data[8]) << 8) | uint64(data[9]),
		}, data[:headerlen], nil
	case AncillaryTextHeaderType:
		headerlen := (uint16(data[1]) << 8) | uint16(data[2])
		return AncillaryTextHeader{
			Type:   htype,
			Length: headerlen,
			Text:   string(data[3:headerlen]),
		}, data[:headerlen], nil
	case KeyHeaderType:
		return KeyHeader{
			Type:   htype,
			Length: headerlen,
		}, data[:headerlen], nil
	case SegmentIdentificationHeaderType:
		return SegmentIdentificationHeader{
			Type:            htype,
			Length:          headerlen,
			ImageIdentifier: (uint16(data[3]) << 8) | uint16(data[4]),
			SequenceNumber:  (uint16(data[5]) << 8) | uint16(data[6]),
			StartColumn:     (uint16(data[7]) << 8) | uint16(data[8]),
			StartLine:       (uint16(data[9]) << 8) | uint16(data[10]),
			MaxSegment:      (uint16(data[11]) << 8) | uint16(data[12]),
			MaxColumn:       (uint16(data[13]) << 8) | uint16(data[14]),
			MaxRow:          (uint16(data[15]) << 8) | uint16(data[16]),
		}, data[:headerlen], nil
	case NOAASpecificHeaderType:
		return NOAASpecificHeader{
			Type:                    htype,
			Length:                  headerlen,
			Agency:                  string(data[3:7]),
			ProductID:               (uint16(data[7]) << 8) | uint16(data[8]),
			ProductSubID:            (uint16(data[9]) << 8) | uint16(data[10]),
			Parameter:               (uint16(data[11]) << 8) | uint16(data[12]),
			NOAASpecificCompression: data[13],
		}, data[:headerlen], nil
	case HeaderStructureRecordHeaderType:
		return HeaderStructureRecordHeader{
			Type:      htype,
			Length:    headerlen,
			Structure: string(data[3:headerlen]),
		}, data[:headerlen], nil
	case RiceCompressionHeaderType:
		return RiceCompressionHeader{
			Type:               htype,
			Length:             headerlen,
			Flags:              (uint16(data[3]) << 8) | uint16(data[4]),
			PixelsPerBlock:     data[5],
			ScanlinesPerPacket: data[6],
		}, data[:headerlen], nil
	case DCSFilenameHeaderType:
		return DCSFilenameHeader{
			Type:     htype,
			Length:   headerlen,
			Filename: string(data[3:headerlen]),
		}, data[:headerlen], nil
	default:
		return nil, []byte{}, fmt.Errorf("Invalid header type found in header! (type = %d)", htype)
	}
}

func MakeSecondaryHeaders(data []byte, ph PrimaryHeader) ([]SecondaryHeader, []byte, error) {
	var ret []SecondaryHeader
	var rawh []byte

	if uint32(len(data))-ph.AllHeaderLength <= uint32(0) {
		return ret, rawh, fmt.Errorf("Not enough data to make all secondary headers!")
	}

	headerlen := int(ph.AllHeaderLength) - int(ph.Length)

	data = data[ph.Length:]
	for headerlen > 0 {
		if sh, raw, err := getNextHeader(data); err == nil {
			ret = append(ret, sh)
			rawh = append(rawh, raw...)
			headerlen -= int(sh.HeaderLength())
			data = data[sh.HeaderLength():]
		} else {
			return []SecondaryHeader{}, []byte{}, err
		}
	}
	return ret, rawh, nil
}

func MakePrimaryHeader(data []byte) (PrimaryHeader, error) {
	if len(data) < 16 {

		return PrimaryHeader{}, fmt.Errorf("Could not make primary header!")
	}
	return PrimaryHeader{
		Type:            data[0],
		Length:          (uint16(data[1]) << 8) | uint16(data[2]),
		FileType:        data[3],
		AllHeaderLength: (uint32(data[4]) << 24) | (uint32(data[5]) << 16) | (uint32(data[6]) << 8) | uint32(data[7]),
		DataLength:      (uint64(data[8]) << 56) | (uint64(data[9]) << 48) | (uint64(data[10]) << 40) | (uint64(data[11]) << 32) | (uint64(data[12]) << 24) | (uint64(data[13]) << 16) | (uint64(data[14]) << 8) | uint64(data[15]),
	}, nil
}

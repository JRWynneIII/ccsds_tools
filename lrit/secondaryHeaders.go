package lrit

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

func (h DCSFilenameHeader) HeaderType() SecondaryHeaderType {
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

func (h DCSFilenameHeader) HeaderLength() uint16 {
	return h.Length
}

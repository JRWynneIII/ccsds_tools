package lrit

import (
	"archive/zip"
	"bytes"
	"io"

	"github.com/opensatelliteproject/goaec/szwrap"
)

func (l File) ContainsZipArchive() bool {
	nsh := l.FindSecondaryHeader(NOAASpecificHeaderType).(NOAASpecificHeader)
	if l.PrimaryHeader.FileType != 0 && nsh.NOAASpecificCompression == 10 {
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

func (l *File) Unzip() error {
	ret := make(map[string][]byte)
	if zr, err := zip.NewReader(bytes.NewReader(l.Data), int64(len(l.Data))); err == nil {
		for _, file := range zr.File {
			if f, err := file.Open(); err == nil {
				defer f.Close()
				if ret[file.Name], err = io.ReadAll(f); err != nil {
					return err
				}
			} else {
				return err
			}
		}
		l.UnzippedData = ret
		return nil
	} else {
		return err
	}

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

//func (l *File) RiceDecompress() error {
//	rch := l.FindSecondaryHeader(RiceCompressionHeaderType).(RiceCompressionHeader)
//	ish := l.FindSecondaryHeader(ImageStructureHeaderType).(ImageStructureHeader)
//	pixels := rch.PixelsPerBlock
//	flags := rch.Flags
//	cols := ish.NumCols
//	if data, err := szwrap.NOAADecompress(l.Data, int(ish.BitsPerPixel), int(pixels), int(cols), int(flags)); err == nil {
//		l.Data = data
//	} else {
//		return err
//	}
//
//	return nil
//}

package lrit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/jrwynneiii/ccsds_tools/packets"
)

func OpenNew(sdu *packets.MSDU) (*File, error) {
	if !sdu.CRCGood {
		//If we don't have a valid CRC, drop this SDU
		return nil, fmt.Errorf("CRC Mismatch in OpenNew()")
	}

	f := &File{
		UnzippedData: make(map[string][]byte),
		RawData:      sdu.Data[10:], //Strip out the Transport header!
		VCDUVersion:  sdu.VCDUVersion,
		VCID:         sdu.VCID,
		CRCGood:      true,
	}

	var err error
	if f.PrimaryHeader, err = MakePrimaryHeader(f.RawData); err == nil {
		f.PrimaryHeaderPopulated = true
	}

	if !f.PrimaryHeader.IsValid() {
		return f, fmt.Errorf("Invalid primary header found: %##v", f.PrimaryHeader)
	}

	if f.PrimaryHeaderPopulated && sdu.Header.SecondaryHeaderFlag {
		if f.SecondaryHeaders, f.RawHeaders, err = MakeSecondaryHeaders(f.RawData, f.PrimaryHeader); err == nil {
			f.SecondaryHeadersPopulated = true
		}
	}

	return f, nil
}

func (f *File) RiceDecompressIfNeededAndAppendBuffer(data []byte) ([]byte, error) {
	if len(data) > 0 {
		var err error
		needsDecompress := false
		var ish ImageStructureHeader
		if f.HeadersPopulated() && f.IsImageFile() {
			if ish, err = f.GetImageStructureHeader(); err == nil {
				if nsh, err := f.GetNOAASpecificHeader(); err == nil {
					if ish.IsCompressed == 1 && nsh.NOAASpecificCompression == 1 {
						needsDecompress = true
					} else {
						log.Infof("File is image, has all headers, but is not compressed")
					}
				} else {
					log.Errorf("Could not find NSH")
				}
			} else {
				log.Errorf("Could not find ISH")
			}
		}

		if needsDecompress {
			if rch, err := f.GetRiceCompressionHeader(); err == nil {
				if d, err := RiceDecompressBuffer(data, rch, ish); err == nil {
					return d, nil
				} else {
					//If decompression fails, just insert black lines
					return []byte{}, fmt.Errorf("Rice decompress failed: %s", err.Error())
				}
			} else {
				//Has no rch
				return []byte{}, fmt.Errorf("LRIT file is Rice compressed image, but has no Rice Compression header!")
			}
		} else {
			return data, fmt.Errorf("LRIT file does not have all headers yet or is not an image")
		}
	}
	return data, fmt.Errorf("Can't decompress: not enough data")
}

func (f *File) Append(sdu *packets.MSDU) error {
	if !sdu.CRCGood {
		if f.PrimaryHeaderPopulated && f.SecondaryHeadersPopulated {
			if !f.IsImageFile() {
				log.Warnf("<ASSEMBLER> Detected CRC mismatch in SDU for packet.")
				return fmt.Errorf("CRC Mismatch in Append()")
			} else {
				log.Warnf("Found CRC mismatch in SDU, but file is an image, so we're attempting to continue")
			}
		} else {
			//If we don't have a valid CRC, drop this SDU
			return fmt.Errorf("CRC Mismatch in Append()")
		}
	}

	var err error

	//Attempt to make secondary header objs
	if !f.SecondaryHeadersPopulated {
		if f.SecondaryHeaders, f.RawHeaders, err = MakeSecondaryHeaders(f.RawData, f.PrimaryHeader); err == nil {
			f.SecondaryHeadersPopulated = true
			var remaining []byte
			if len(f.RawData) > int(f.PrimaryHeader.AllHeaderLength) {
				remaining = f.RawData[f.PrimaryHeader.AllHeaderLength:]
				f.RawData = f.RawData[:f.PrimaryHeader.AllHeaderLength]
			}
			if d, err := f.RiceDecompressIfNeededAndAppendBuffer(remaining); err == nil {
				f.RawData = append(f.RawData, d...)
			} else {
				log.Error(err)
				f.RawData = append(f.RawData, remaining...)
			}
		}
	}

	//Try to decompress the sdu if we already know its an image file
	if d, err := f.RiceDecompressIfNeededAndAppendBuffer(sdu.Data); err == nil {
		f.RawData = append(f.RawData, d...)
	} else {
		log.Error(err)
		f.RawData = append(f.RawData, sdu.Data...)
	}

	return nil
}

func (f *File) MissingRows() uint64 {
	datalen := uint64(len(f.Data))
	if len(f.Data) == 0 {
		datalen = uint64(len(f.RawData[f.PrimaryHeader.AllHeaderLength:]))
	}
	if f.IsImageFile() {
		if ish, err := f.GetImageStructureHeader(); err == nil {
			missingBytes := (f.PrimaryHeader.DataLength / 8) - datalen
			missingRows := missingBytes / uint64(ish.NumCols)
			return missingRows
		}
	}
	return 0
}

func (f *File) GetFillRow() []byte {
	if f.IsImageFile() {
		if ish, err := f.GetImageStructureHeader(); err == nil {
			if len(f.Data) == 0 && uint32(len(f.RawData)) > uint32(ish.NumCols)+f.PrimaryHeader.AllHeaderLength {
				return f.RawData[len(f.RawData)-int(ish.NumCols):]
			}

			if len(f.Data) > int(ish.NumCols) {
				return f.Data[len(f.Data)-int(ish.NumCols):]
			} else {
				return make([]byte, ish.NumCols)
			}
		}
	}
	return []byte{}

}

func (f *File) Close() error {
	if !f.HeadersPopulated() {
		log.Errorf("PH: %##v\tSH:%##v", f.PrimaryHeader, f.SecondaryHeaders)
		return fmt.Errorf("Invalid LRIT file! Could not create headers")
	}

	f.Data = f.RawData[f.PrimaryHeader.AllHeaderLength:]

	if f.IsImageFile() {
		if ish, err := f.GetImageStructureHeader(); err == nil {
			missingBytes := (f.PrimaryHeader.DataLength / 8) - uint64(len(f.Data))
			missingRows := missingBytes / uint64(ish.NumCols)
			log.Debugf("Expected len: %d, Actual len: %d, expected rows: %d, got rows: %d, expected cols: %d",
				f.PrimaryHeader.DataLength/8, len(f.Data), ish.NumRows, len(f.Data)/int(ish.NumRows), ish.NumCols)
			log.Debugf("Missing bytes: %d, missing rows: %d", missingBytes, missingRows)
			if missingRows < uint64(ish.NumRows) && missingRows > 0 {
				for i := uint64(0); i < missingRows; i++ {
					log.Debugf("Filling image with %d pixels", ish.NumCols)
					f.RawData = append(f.RawData, f.GetFillRow()...)
					f.Data = append(f.Data, f.GetFillRow()...)
				}
			}
		}
	}
	log.Debug("Finished filling file and closing")

	return nil
}

func (f File) HeadersPopulated() bool {
	if !f.PrimaryHeaderPopulated {
		return false
	} else if !f.PrimaryHeader.IsValid() {
		return false
	}

	if !f.SecondaryHeadersPopulated {
		return false
	}

	return true
}

func (l File) WriteFile(dir string) {
	filenamefull := l.GetName()
	productID := l.FindSecondaryHeader(NOAASpecificHeaderType).(NOAASpecificHeader).ProductID
	if l.IsImageFile() && productID >= 16 && productID <= 19 {
		tmp := l.FindSecondaryHeader(SegmentIdentificationHeaderType)
		if tmp != nil {
			sih := tmp.(SegmentIdentificationHeader)
			filename := strings.TrimSuffix(filenamefull, ".lrit")
			filenamefull = fmt.Sprintf("%s_%03d.lrit", filename, sih.SequenceNumber)
		} else {
			path := filepath.Join(dir, filenamefull)

			if err := os.WriteFile(path, l.RawData, os.FileMode(0644)); err != nil {
				log.Errorf("Could not write file; was not a segmented image file %s", path)
			}
			return
		}
	}

	if l.ContainsZipArchive() {
		if err := l.Unzip(); err == nil {
			for name, data := range l.UnzippedData {
				path := filepath.Join(dir, name)
				if err = os.WriteFile(path, data, os.FileMode(0644)); err != nil {
					log.Errorf("Could not write unzipped file: %s from LRIT file %s", name, filenamefull)
				}
			}
		} else {
			log.Error(err)
		}
	} else {
		path := filepath.Join(dir, filenamefull)

		if err := os.WriteFile(path, l.RawData, os.FileMode(0644)); err != nil {
			log.Errorf("Could not write file %s", path)
		}
	}
}

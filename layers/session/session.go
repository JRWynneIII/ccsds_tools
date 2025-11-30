package session

import (
	"time"

	"github.com/charmbracelet/log"
	"github.com/jrwynneiii/ccsds_tools/lrit"
	"github.com/jrwynneiii/ccsds_tools/packets"
)

type LRITGen struct {
	TransportInput *chan *packets.TransportFile
	LRITOutput     *chan *lrit.File
}

func New(input *chan *packets.TransportFile, output *chan *lrit.File) *LRITGen {
	return &LRITGen{
		TransportInput: input,
		LRITOutput:     output,
	}
}

func (t *LRITGen) Start() {
	for {
		select {
		case tpfile := <-*t.TransportInput:
			t.ProcessTransportFile(tpfile)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func (l *LRITGen) ProcessTransportFile(t *packets.TransportFile) {
	if lf, err := lrit.NewLRITFile(t.Version, t.VCDUVersion, t.Data, t.CRCGood, t.VCID); err == nil {
		if valid, err := lf.IsValid(); !valid {
			switch err {
			case lrit.LRITPrimaryHeaderErr:
				log.Error(err)
				return
			case lrit.LRITLengthMismatchErr:
				log.Errorf("%s. Have: %d, Want: %d", err.Error(), len(lf.Data), lf.PrimaryHeader.DataLength/8)
				return
			case lrit.LRITCRCMismatchErr:
				if lf.IsImageFile() {
					log.Warnf("LRIT file %s has CRC mismatch, but attempting to continue...", lf.GetName())
				} else {
					log.Errorf("LRIT file has CRC mismatch! Dropping...")
					return
				}
			}
		}

		if err := l.DecompressIfNeeded(lf); err != nil {
			log.Errorf("LRIT file contains ZIP archive, but failed to decompress: %s", err.Error())
			return
		}

		*l.LRITOutput <- lf
	} else {
		log.Error(err)
	}
}

func (l *LRITGen) DecompressIfNeeded(lf *lrit.File) error {
	if lf.ContainsZipArchive() {
		return lf.Unzip()
	}
	return nil
}

// Boilerplate to satisfy interface
func (t *LRITGen) Destroy() {
}

func (t *LRITGen) GetInput() any {
	return t.TransportInput
}

func (t *LRITGen) GetOutput() any {
	return t.LRITOutput
}

func (t *LRITGen) Reset() {
}

func (t *LRITGen) Flush() {
}

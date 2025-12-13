package session

import (
	"time"

	"github.com/charmbracelet/log"
	"github.com/jrwynneiii/ccsds_tools/lrit"
)

type LRITGen struct {
	TransportInput *chan lrit.File
	LRITOutput     *chan *lrit.File
}

func New(input *chan lrit.File, output *chan *lrit.File) *LRITGen {
	return &LRITGen{
		TransportInput: input,
		LRITOutput:     output,
	}
}

func (t *LRITGen) Start() {
	for {
		select {
		case tpfile := <-*t.TransportInput:
			t.ProcessTransportFile(&tpfile)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func (l *LRITGen) ProcessTransportFile(lf *lrit.File) {
	if valid, err := lf.IsValid(); !valid {
		switch err {
		case lrit.LRITPrimaryHeaderErr:
			log.Error(err)
			return
		case lrit.LRITLengthMismatchErr:
			log.Errorf("(%s) %s. Have: %d, Want: %d", lf.GetName(), err.Error(), len(lf.Data), lf.PrimaryHeader.DataLength/8)
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

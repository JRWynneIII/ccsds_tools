package session

import (
	"slices"
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
	//l.dumpTransportFile(t)
	lf := lrit.File{
		Version:     t.Version,
		VCDUVersion: t.VCDUVersion,
		Data:        t.Data,
		//	HaveCRC:     t.HaveCRC,
		//	WantCRC:     t.WantCRC,
		RawData: t.Data,
		VCID:    t.VCID,
		CRCGood: t.CRCGood,
	}
	ph := lrit.MakePrimaryHeader(lf.Data)

	lf.PrimaryHeader = ph
	lf.Data = lf.Data[16:]
	if !ph.IsValid() {
		log.Errorf("Invalid LRIT primary header detected! %##v", ph)
		return
	}

	if err := lf.PopulateSecondaryHeaders(); err != nil {
		log.Error(err)
	}

	if valid, err := lf.IsValid(); !valid {
		switch err {
		case lrit.LRITPrimaryHeaderErr:
			log.Error(err)
			return
		case lrit.LRITLengthMismatchErr:
			log.Errorf("%s. Have: %d, Want: %d", err.Error(), len(lf.Data), lf.PrimaryHeader.DataLength/8)
			return
		case lrit.LRITCRCMismatchErr:
			hasImgStructHeader := slices.ContainsFunc(lf.SecondaryHeaders, func(a lrit.SecondaryHeader) bool {
				return a.HeaderType() == lrit.ImageStructureHeaderType
			})
			if lf.PrimaryHeader.FileType == 0 && hasImgStructHeader {
				log.Warnf("LRIT file %s has CRC mismatch, but attempting to continue...", lf.GetName())
			} else {
				log.Errorf("LRIT file has CRC mismatch! Dropping...")
				return
			}
		}
	}

	*l.LRITOutput <- &lf
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

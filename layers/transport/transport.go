package transport

import (
	"slices"
	"time"

	"github.com/charmbracelet/log"
	"github.com/jrwynneiii/ccsds_tools/lrit"
)

type TransportLayer struct {
	FramesInput     *chan []byte
	TransportOutput *chan lrit.File

	Assemblers      map[uint8]*TransportAssembler
	IgnoredChannels []uint8

	ContinueOnCRCFailure   bool
	FillMissingSDUWithNull bool
}

func New(input *chan []byte, output *chan lrit.File) *TransportLayer {
	return &TransportLayer{
		FramesInput:            input,
		TransportOutput:        output,
		Assemblers:             make(map[uint8]*TransportAssembler),
		ContinueOnCRCFailure:   false,
		FillMissingSDUWithNull: true,
	}
}

func (t *TransportLayer) IgnoreChannel(id int) {
	if !slices.Contains(t.IgnoredChannels, uint8(id)) {
		t.IgnoredChannels = append(t.IgnoredChannels, uint8(id))
	}
}

func (t *TransportLayer) Start() {
	t.IgnoreChannel(63)
	for {
		select {
		case frame := <-*t.FramesInput:
			t.ProcessFrame(frame)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func (t *TransportLayer) ProcessFrame(data []byte) {
	//Create our transport assembler if it doesn't exist
	vcid := uint8(data[1]) & 0x3f
	if t.Assemblers[vcid] == nil {
		t.Assemblers[vcid] = NewTransportAssembler(t.TransportOutput, vcid)
	}

	if vcdu, err := t.Assemblers[vcid].ParseFrame(data); err == nil {
		if !slices.Contains(t.IgnoredChannels, vcdu.VCID) {
			t.Assemblers[vcid].ProcessVCDU(vcdu)
		}
	} else {
		log.Error(err)
	}
}

// Boilerplate to satisfy interface
func (t *TransportLayer) Destroy() {
}

func (t *TransportLayer) GetInput() any {
	return t.FramesInput
}

func (t *TransportLayer) GetOutput() any {
	return t.TransportOutput
}

func (t *TransportLayer) Reset() {
}

func (t *TransportLayer) Flush() {
}

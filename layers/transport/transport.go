package transport

import (
	"time"

	"github.com/jrwynneiii/ccsds_tools/packets"
)

type PacketAssembler struct {
	FramesInput *chan []byte
	LRITOutput  *chan *packets.LRITFile
	//map[ChannelID] = VCDU
	VCDUsByChannel map[int][]*packets.VCDU

	//map[ChannelID][APID] = CP_PDU
	SortedCP_PDUs map[int]map[int][]*packets.CP_PDU
}

func New(input *chan []byte, output *chan *packets.LRITFile) *PacketAssembler {
	return &PacketAssembler{
		FramesInput:    input,
		LRITOutput:     output,
		VCDUsByChannel: make(map[int][]*packets.VCDU),
	}
}

func (p *PacketAssembler) Start() {
	for {
		select {
		case frame := <-*p.FramesInput:
			p.ProcessFrame(frame)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

// Process:
// 	* Parse frame into a VCDU.
//	* Sort VCDUs by VCID/Channel
//	* Per Channel:
//		* Construct CP_PDUs from one or more VCDUs
//		* Sort CP_PDUs by APID
//		* Per APID:
//			* Construct LRIT Blocks from one or more CP_PDUs
//			* Checksum blocks
//			* Discard if bad, or try and recover somehow?
//			* Construct LRIT File from one or more LRITBlocks
//			* Send LRITFile to next layer

func (p *PacketAssembler) ProcessFrame(data []byte) {
	packet := packets.ParseFrame(data)
	//Sort packet into bins for channels
	if packet.Header.VCID != 63 {
		p.VCDUsByChannel[packet.Header.VCID] = append(p.VCDUsByChannel[packet.Header.VCID], packet)
	} else {
		//New CADU, should we clear(p.Channels)?
	}
}

func (p *PacketAssembler) Destroy() {
}

func (p *PacketAssembler) GetInput() any {
	return p.FramesInput
}

func (p *PacketAssembler) GetOutput() any {
	return p.LRITOutput
}

func (p *PacketAssembler) Reset() {
}

func (p *PacketAssembler) Flush() {
}

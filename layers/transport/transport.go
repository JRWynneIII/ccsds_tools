package transport

import (
	"slices"
	"sort"
	"time"

	"github.com/charmbracelet/log"
	"github.com/jrwynneiii/ccsds_tools/packets"
)

type TransportLayer struct {
	FramesInput *chan []byte
	DebugOutput *chan *packets.TPPDU
	LRITOutput  *chan *packets.LRITFile

	Assemblers        map[uint8]*TransportAssembler
	TransportFiles    map[uint8]map[uint16][]*packets.TPPDU
	IncompletePackets map[uint8]map[uint16][]*packets.CPPDU
	IgnoredChannels   []uint8
}

func New(input *chan []byte, output *chan *packets.LRITFile) *TransportLayer {
	debug := make(chan *packets.TPPDU)
	return &TransportLayer{
		FramesInput:       input,
		LRITOutput:        output,
		DebugOutput:       &debug,
		Assemblers:        make(map[uint8]*TransportAssembler),
		TransportFiles:    make(map[uint8]map[uint16][]*packets.TPPDU),
		IncompletePackets: make(map[uint8]map[uint16][]*packets.CPPDU),
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
		t.Assemblers[vcid] = NewTransportAssembler()
	}

	if vcdu, err := t.Assemblers[vcid].ParseFrame(data); err == nil {
		if !slices.Contains(t.IgnoredChannels, vcdu.VCID) {
			t.ProcessVCDU(vcdu)
		}
	} else {
		log.Error(err)
	}
}

//NOTE: MSDU's might need to be built over frames from each channel, not from each VCDU.
// E.g. if we only have a partial msdu at the end of VCDU{vcid:2}, and the next VCDU.vcid == 4,
//	Then don't build a msdu from the next packet, but the next packet from vcid==2!
//	Verify this in satdump and goestools!

func (t *TransportLayer) ClearIncompletePacketBuffer(vcid uint8, apid uint16) {
	if t.IncompletePackets[vcid] != nil {
		t.IncompletePackets[vcid][apid] = []*packets.CPPDU{}
	}
}

func (t *TransportLayer) InsertIncompletePacket(vcid uint8, packet *packets.CPPDU) {
	if t.IncompletePackets[vcid] == nil {
		t.IncompletePackets[vcid] = make(map[uint16][]*packets.CPPDU)
	}
	t.IncompletePackets[vcid][packet.Header.APID] = append(t.IncompletePackets[vcid][packet.Header.APID], packet)
}

func (l *TransportLayer) CreateTransportFile(sdus []*packets.CPPDU) *packets.TPPDU {
	t := packets.TPPDU{}

	var data []byte
	var CRC uint16

	sort.Slice(sdus, func(a, b int) bool {
		return sdus[a].Header.PacketSequenceCounter < sdus[b].Header.PacketSequenceCounter
	})

	for idx, sdu := range sdus {
		if idx > 0 {
			if packets.CounterDiff(16384, uint32(sdus[idx-1].Header.PacketSequenceCounter), uint32(sdu.Header.PacketSequenceCounter))-1 > 0 {
				log.Errorf("Missing SDU! Last packet: %d, current packet: %d", sdus[idx-1].Header.PacketSequenceCounter, sdu.Header.PacketSequenceCounter)
				//TODO: Add null bytes when missing chunk
			}
		}
		data = append(data, sdu.Data...)
	}

	CRC = (uint16(data[len(data)-2]) << 8) | uint16(data[len(data)-1])
	data = data[:len(data)-2]
	t.Data = data
	t.CalcCRC()

	var err error
	t.Header, err = MakeTPPDUHeader(data)
	if err != nil {
		log.Fatal(err)
	} else {
		data = data[10:] // Shift past header
	}

	t.VCDUVersion = sdus[len(sdus)-1].VCDUVersion
	t.VCDUSCID = sdus[len(sdus)-1].VCDUSCID
	t.VCID = sdus[len(sdus)-1].VCID
	t.VCDUCounter = sdus[len(sdus)-1].VCDUCounter
	t.VCDUReplay = sdus[len(sdus)-1].VCDUReplay
	t.Version = sdus[len(sdus)-1].Header.Version
	t.Type = sdus[len(sdus)-1].Header.Type
	t.SecondaryHeaderFlag = sdus[len(sdus)-1].Header.SecondaryHeaderFlag
	t.APID = sdus[len(sdus)-1].Header.APID
	t.SequenceFlag = sdus[len(sdus)-1].Header.SequenceFlag
	t.PacketSequenceCounter = sdus[len(sdus)-1].Header.PacketSequenceCounter
	t.PacketLength = sdus[len(sdus)-1].Header.PacketLength
	t.HaveCRC = CRC
	t.Data = data

	return &t
}

func (t *TransportLayer) ProcessVCDU(vcdu *packets.VCDU) {
	if sdus, err := t.Assemblers[vcdu.VCID].ParseMSDUs(vcdu); err != nil {
		log.Error(err)
	} else {
		if !vcdu.IsCorrupt {
			for _, sdu := range sdus {
				if sdu.Header.APID == 2047 {
					//Drop fill packets
					continue
				}

				sdu.VCDUVersion = vcdu.VCDUVersion
				sdu.VCDUSCID = vcdu.VCDUSCID
				sdu.VCID = vcdu.VCID
				sdu.VCDUCounter = vcdu.VCDUCounter
				sdu.VCDUReplay = vcdu.VCDUReplay

				switch sdu.Header.SequenceFlag {
				case 0:
					//Continuation of last packet
					t.InsertIncompletePacket(vcdu.VCID, &sdu)
				case 1:
					//Start new packet
					//t.ClearIncompletePacketBuffer(vcdu.VCID, sdu.Header.APID)
					t.InsertIncompletePacket(vcdu.VCID, &sdu)
				case 2:
					//End packet
					t.InsertIncompletePacket(vcdu.VCID, &sdu)
					tppdu := t.CreateTransportFile(t.IncompletePackets[vcdu.VCID][sdu.Header.APID])
					//Clear out the buffer of incomplete packets
					t.ClearIncompletePacketBuffer(vcdu.VCID, sdu.Header.APID)
					if tppdu != nil {
						*t.DebugOutput <- tppdu
					}
				case 3:
					//Self contained packet
					tppdu := t.CreateTransportFile([]*packets.CPPDU{&sdu})
					if tppdu != nil {
						*t.DebugOutput <- tppdu
					}
				default:
					log.Errorf("Invalid sequence flag: %d", sdu.Header.SequenceFlag)
				}
			}
		}
	}
}

// Boilerplate to satisfy interface
func (t *TransportLayer) Destroy() {
}

func (t *TransportLayer) GetInput() any {
	return t.FramesInput
}

func (t *TransportLayer) GetOutput() any {
	return t.LRITOutput
}

func (t *TransportLayer) Reset() {
}

func (t *TransportLayer) Flush() {
}

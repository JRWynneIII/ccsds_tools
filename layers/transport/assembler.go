package transport

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/jrwynneiii/ccsds_tools/packets"
)

type TransportAssembler struct {
	lastAPID        uint16
	lastVCDUCounter uint32
	lastSDU         []byte
	lastVCDU        *packets.VCDU
}

func NewTransportAssembler() *TransportAssembler {
	return &TransportAssembler{0, 0, []byte{}, nil}
}

func MakeMSDUHeader(data []byte) (packets.CPPDUHeader, error) {
	h := packets.CPPDUHeader{}
	if len(data) < 6 {
		return h, fmt.Errorf("Not enough data for header! Need 6 hytes")
	}
	h.Version = ((data[0] >> 5) & 0x7)
	h.Type = ((data[0] >> 4) & 0x1) > 0
	h.SecondaryHeaderFlag = ((data[0] >> 3) & 0x1) > 0
	h.APID = ((uint16(data[0]) & 0x7) << 8) | uint16(data[1])
	h.SequenceFlag = (data[2] >> 6) & 0x3
	h.PacketSequenceCounter = ((uint16(data[2]) & 0x3f) << 8) | uint16(data[3])
	h.PacketLength = ((uint16(data[4]) << 8) | uint16(data[5])) + 1

	return h, nil
}

func MakeTPPDUHeader(data []byte) (packets.TPPDUHeader, error) {
	if len(data) < 10 {
		return packets.TPPDUHeader{}, fmt.Errorf("data is too short for tppdu header creation")
	}

	counter := (uint16(data[0]) << 8) | uint16(data[1])
	length := (uint64(data[2]) << 56) | (uint64(data[3]) << 48) | (uint64(data[4]) << 40) | (uint64(data[5]) << 32) | (uint64(data[6]) << 24) | (uint64(data[7]) << 16) | (uint64(data[8]) << 8) | uint64(data[9])
	return packets.TPPDUHeader{
		Counter: counter,
		Length:  length,
	}, nil
}

func (t *TransportAssembler) checkForSkippedVCDU(vcdu *packets.VCDU) error {
	var err error
	if t.lastVCDU != nil {
		if packets.CounterDiff(1<<24, t.lastVCDU.VCDUCounter, vcdu.VCDUCounter) > 1 {
			err = fmt.Errorf("Dropped VCDU found! Last packet: %d, current packet: %d", t.lastVCDU.VCDUCounter, vcdu.VCDUCounter)
			if t.lastVCDU.VCDUCounter == vcdu.VCDUCounter && vcdu.VCDUVersion == t.lastVCDU.VCDUVersion {
				err = fmt.Errorf("Duplicate VCDU found! Last packet: %d, current packet: %d", t.lastVCDU.VCDUCounter, vcdu.VCDUCounter)
			}
		}
	}
	return err
}

func (t *TransportAssembler) saveContinuationPacket(vcdu *packets.VCDU) {
	if len(t.lastSDU) > 0 {
		t.lastSDU = append(t.lastSDU, vcdu.Data...)
	}
	t.lastVCDU = vcdu
}

func (t *TransportAssembler) savePartialPacketEnd(fhp uint16, data []byte) []byte {
	if fhp > 0 && len(t.lastSDU) > 0 {
		t.lastSDU = append(t.lastSDU, data[:fhp]...)
	}
	return data[fhp:]
}

func (t *TransportAssembler) ParseMSDUs(vcdu *packets.VCDU) ([]packets.CPPDU, error) {
	err := t.checkForSkippedVCDU(vcdu)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate VCDU found!") {
			t.lastVCDU = vcdu
			return nil, err
		}
		log.Error(err)
	}

	var ret []packets.CPPDU

	// If we get a packet without a header, just append all the data
	if !vcdu.ContainsMSDUHeader() {
		t.saveContinuationPacket(vcdu)
		return ret, nil
	}

	// Pulling data out of the packet so that its easier to work with, and we don't muss up the packet's data frame
	data := vcdu.Data

	//Check to make sure fhp is within the packet
	if err = vcdu.FHPIsValid(); err != nil {
		t.lastVCDU = vcdu
		return ret, err
	}

	// If we have some data before our first header, save it and shift
	data = t.savePartialPacketEnd(vcdu.FirstHeaderOffset, data)

	// If we have enough for a header, lets go ahead and make one. If not, we probably missed somehting so drop it
	if len(t.lastSDU) > 6 {
		if header, err := MakeMSDUHeader(t.lastSDU); err != nil {
			// Could not create a header for some reason, so lets bail
			log.Error(err)
			t.lastSDU = []byte{}
		} else {
			// If its not a fill cppdu
			if !header.IsFillPacket() {
				c := packets.CPPDU{
					Header:      header,
					VCDUVersion: t.lastVCDU.VCDUVersion,
					VCDUSCID:    t.lastVCDU.VCDUSCID,
					VCID:        t.lastVCDU.VCID,
					VCDUCounter: t.lastVCDU.VCDUCounter,
					VCDUReplay:  t.lastVCDU.VCDUReplay,
					Data:        t.lastSDU[6:],
				}
				ret = append(ret, c)
			}
			t.lastSDU = []byte{}
		}
	}

	t.lastSDU = []byte{}

	for len(data) > 6 {
		if header, err := MakeMSDUHeader(data); err != nil {
			log.Error(err)
			data = []byte{}
			vcdu.IsCorrupt = true
			break
		} else {
			dataWithHeader := data
			// Move to end of header/beginning of pkt
			data = data[6:]

			//If we've got a fill APID, ignore it and continue; shift data to delete the packets
			if header.IsFillPacket() {
				if header.PacketLength > uint16(len(data)) {
					log.Errorf("Fill APID packet has incorrect packet length %d! Can't decode past this pkt", header.PacketLength)
					data = []byte{}
					t.lastVCDU = vcdu
					break
				}
				data = data[header.PacketLength:]
				continue
			}

			// If we don't have a full cppdu, but have enough for a header, save the bytes, with the header
			// This means we're probably at the end of the VCDU packet zone
			if header.PacketLength > uint16(len(data)) {
				//Save header and data
				t.lastSDU = append(t.lastSDU, dataWithHeader...)
				data = []byte{} // emptying data so that we don't accidentally save it again
				break
			}

			c := packets.CPPDU{
				Header:      header,
				Data:        data[:header.PacketLength],
				VCDUVersion: vcdu.VCDUVersion,
				VCDUSCID:    vcdu.VCDUSCID,
				VCID:        vcdu.VCID,
				VCDUCounter: vcdu.VCDUCounter,
				VCDUReplay:  vcdu.VCDUReplay,
			}

			ret = append(ret, c)
			data = data[header.PacketLength:]
		}
	}

	// If we have less than a header's worth of bytes left, save it
	if len(data) > 0 && !vcdu.IsCorrupt {
		t.lastSDU = append(t.lastSDU, data...)
	}

	t.lastVCDU = vcdu

	return ret, nil
}

func (t *TransportAssembler) ParseFrame(data []byte) (*packets.VCDU, error) {
	if !packets.FrameIsValid(data) {
		return nil, fmt.Errorf("Bad frame size! Have: %d want: %d", len(data), 892)
	}

	version := (data[0] & 0xc0) >> 6
	scid := ((data[0] & 0x3f) << 2) | ((data[1] & 0xc0) >> 6)
	vcid := (data[1] & 0x3f)
	counter := (uint32(data[2]) << 16) | (uint32(data[3]) << 8) | uint32(data[4])
	replay := ((data[5] & 0b10000000) >> 7) > 0
	data = data[6:]

	//TODO: Remove; this is just a sanity check
	if len(data) != 886 {
		log.Errorf("Header too large? VCDU Data zone size = %d; want 886", len(data))
	}

	//fhp := (binary.BigEndian.Uint16(data[:2]) & 0b0000111111111111)
	fhp := ((uint16(data[0]) & 0x7) << 8) | uint16(data[1])
	data = data[2:]

	//TODO: Remove; this is just a sanity check
	if len(data) != 884 {
		log.Errorf("MPDU Packet zone too large/small! Got: %d want 884", len(data))
	}

	v := packets.VCDU{
		VCDUVersion:       version,
		VCDUSCID:          scid,
		VCID:              vcid,
		VCDUCounter:       counter,
		VCDUReplay:        replay,
		FirstHeaderOffset: fhp,
		IsCorrupt:         false,
		Data:              data,
	}
	return &v, nil
}

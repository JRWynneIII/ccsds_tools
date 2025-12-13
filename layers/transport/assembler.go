package transport

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/log"
	"github.com/jrwynneiii/ccsds_tools/lrit"
	"github.com/jrwynneiii/ccsds_tools/packets"
)

type TransportAssembler struct {
	lastVCDUCounter uint32
	lastSDU         []byte
	lastVCDU        *packets.VCDU
	APIDs           map[uint16][]*packets.MSDU
	Files           map[uint16]*lrit.File
	TransportOutput *chan lrit.File
	VCID            uint8
	lastAppliedSDU  map[uint16]*packets.MSDU
}

func NewTransportAssembler(output *chan lrit.File, vcid uint8) *TransportAssembler {
	return &TransportAssembler{
		lastVCDUCounter: 0,
		lastSDU:         []byte{},
		lastVCDU:        nil,
		APIDs:           make(map[uint16][]*packets.MSDU),
		Files:           make(map[uint16]*lrit.File),
		TransportOutput: output,
		VCID:            vcid,
		lastAppliedSDU:  make(map[uint16]*packets.MSDU),
	}
}

func (t *TransportAssembler) Drop(apid uint16) {
	delete(t.APIDs, apid)
	delete(t.lastAppliedSDU, apid)
	delete(t.Files, apid)
}

func (t *TransportAssembler) processAnySkippedSDU(apid uint16, sdu *packets.MSDU) {
	diff := uint(0)
	if t.lastAppliedSDU[apid] != nil {
		if diff = packets.CounterDiff(16384, uint32(t.lastAppliedSDU[apid].Header.PacketSequenceCounter), uint32(sdu.Header.PacketSequenceCounter)) - 1; diff > 0 {
			log.Infof("Missing SDU! Last packet: %d, current packet: %d", t.lastAppliedSDU[apid].Header.PacketSequenceCounter, sdu.Header.PacketSequenceCounter)
			var ish lrit.ImageStructureHeader
			if t.Files[apid] != nil {
				if t.Files[apid].SecondaryHeadersPopulated && t.Files[apid].IsImageFile() {
					ish, _ = t.Files[apid].GetImageStructureHeader()
					if ish != (lrit.ImageStructureHeader{}) {
						if diff > uint(t.Files[apid].MissingRows()) {
							log.Error("Dropping file %s, due to skipped end rows", t.Files[apid].GetName())
							t.Drop(apid)
						} else {
							for i := uint(0); i < diff; i++ {
								log.Infof("Filling missing packets...")
								t.Files[apid].RawData = append(t.Files[apid].RawData, t.Files[apid].GetFillRow()...)
							}
						}
					}
				} else {
					log.Errorf("Dropping LRIT file; missing %d SDUs and is not an image", diff)
					t.Drop(apid)
				}
			}
		}
	}
}

func (t *TransportAssembler) ProcessAllAPIDs() {
	for apid, sdus := range t.APIDs {
		sort.Slice(sdus, func(a, b int) bool {
			return sdus[a].Header.PacketSequenceCounter < sdus[b].Header.PacketSequenceCounter
		})
		for _, sdu := range sdus {
			if len(sdu.Data) < 2 {
				continue
			}

			t.processAnySkippedSDU(apid, sdu)
			t.lastAppliedSDU[apid] = sdu

			CRC := (uint16(sdu.Data[len(sdu.Data)-2]) << 8) | uint16(sdu.Data[len(sdu.Data)-1])
			sdu.Data = sdu.Data[:len(sdu.Data)-2]

			calcCRC := packets.CalcCRCBuffer(sdu.Data)
			if calcCRC != CRC {
				t.Drop(apid)
				log.Error("CRC Mismatch")
			} else {
				sdu.CRCGood = true
			}

			switch sdu.Header.SequenceFlag {
			case 0:
				//Continuation of last packet
				if t.Files[apid] != nil {
					if err := t.Files[apid].Append(sdu); err != nil {
						log.Error(err)
						t.Drop(apid)
					}
				}
			case 1:
				//Start new packet
				//Clear out any existing file; like if we started and got garbage
				t.Drop(apid)
				var err error
				if t.Files[apid], err = lrit.OpenNew(sdu); err != nil {
					log.Error(err)
				}
			case 2:
				//End packet
				if t.Files[apid] != nil {
					if err := t.Files[apid].Append(sdu); err != nil {
						log.Error(err)
						t.Drop(apid)
						continue
					}
					if err := t.Files[apid].Close(); err != nil {
						log.Error(err)
						t.Drop(apid)
						continue
					}

					*t.TransportOutput <- *t.Files[apid]
					//Clear out the buffer
					t.Drop(apid)
				}
			case 3:
				//Self contained packet
				t.Drop(apid)
				var err error
				if t.Files[apid], err = lrit.OpenNew(sdu); err != nil {
					log.Error(err)
					t.Drop(apid)
					continue
				}

				if err := t.Files[apid].Close(); err != nil {
					log.Error(err)
					t.Drop(apid)
					continue
				}

				*t.TransportOutput <- *t.Files[apid]

				t.Drop(apid)
			default:
				log.Errorf("Invalid sequence flag: %d", sdu.Header.SequenceFlag)
			}
		}
		delete(t.APIDs, apid)
		delete(t.lastAppliedSDU, apid)
	}
}

func MakeMSDUHeader(data []byte) (packets.MSDUHeader, error) {
	h := packets.MSDUHeader{}
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

func (t *TransportAssembler) ParseMSDUs(vcdu *packets.VCDU) error {
	err := t.checkForSkippedVCDU(vcdu)
	if err != nil {
		//l.ClearIncompletePacketBufferByVCID(vcdu.VCID)
		for apid, _ := range t.APIDs {
			delete(t.APIDs, apid)
		}
		//if strings.Contains(err.Error(), "Duplicate VCDU found!") {
		//	t.lastVCDU = vcdu
		//	return err
		//}

		log.Error(err)
	}

	var ret []packets.MSDU

	// If we get a packet without a header, just append all the data
	if !vcdu.ContainsMSDUHeader() {
		t.saveContinuationPacket(vcdu)
		return nil
	}

	// Pulling data out of the packet so that its easier to work with, and we don't muss up the packet's data frame
	data := vcdu.Data

	//Check to make sure fhp is within the packet
	if err = vcdu.FHPIsValid(); err != nil {
		t.lastVCDU = vcdu
		return err
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
				c := packets.MSDU{
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

			c := packets.MSDU{
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

	for _, sdu := range ret {
		if sdu.Header.APID == 2047 {
			//Drop fill packets
			continue
		}

		t.APIDs[sdu.Header.APID] = append(t.APIDs[sdu.Header.APID], &sdu)
	}

	return nil
}

func (t *TransportAssembler) ProcessVCDU(vcdu *packets.VCDU) error {
	if !vcdu.IsCorrupt {
		if err := t.ParseMSDUs(vcdu); err != nil {
			return err
		}
		t.ProcessAllAPIDs()
	}
	return nil
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

	if len(data) != 886 {
		log.Errorf("Header too large? VCDU Data zone size = %d; want 886", len(data))
	}

	fhp := ((uint16(data[0]) & 0x7) << 8) | uint16(data[1])
	data = data[2:]

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

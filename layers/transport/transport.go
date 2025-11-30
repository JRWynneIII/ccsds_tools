package transport

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/charmbracelet/log"
	"github.com/jrwynneiii/ccsds_tools/lrit"
	"github.com/jrwynneiii/ccsds_tools/packets"
)

type TransportLayer struct {
	FramesInput     *chan []byte
	TransportOutput *chan *packets.TransportFile

	Assemblers        map[uint8]*TransportAssembler
	TransportFiles    map[uint8]map[uint16][]*packets.TransportFile
	IncompletePackets map[uint8]map[uint16][]*packets.MSDU
	IgnoredChannels   []uint8

	ContinueOnCRCFailure   bool
	FillMissingSDUWithNull bool
}

func New(input *chan []byte, output *chan *packets.TransportFile) *TransportLayer {
	return &TransportLayer{
		FramesInput:            input,
		TransportOutput:        output,
		Assemblers:             make(map[uint8]*TransportAssembler),
		TransportFiles:         make(map[uint8]map[uint16][]*packets.TransportFile),
		IncompletePackets:      make(map[uint8]map[uint16][]*packets.MSDU),
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

func (t *TransportLayer) ClearIncompletePacketBuffer(vcid uint8, apid uint16) {
	if t.IncompletePackets[vcid] != nil {
		t.IncompletePackets[vcid][apid] = []*packets.MSDU{}
	}
}

func (t *TransportLayer) InsertIncompletePacket(vcid uint8, packet *packets.MSDU) {
	if t.IncompletePackets[vcid] == nil {
		t.IncompletePackets[vcid] = make(map[uint16][]*packets.MSDU)
	}

	t.IncompletePackets[vcid][packet.Header.APID] = append(t.IncompletePackets[vcid][packet.Header.APID], packet)
}

func SDUNeedsDecompress(data []byte) (bool, int, lrit.RiceCompressionHeader, lrit.ImageStructureHeader) {
	needsDecompress := false
	endOfHeaders := 0
	ish := lrit.ImageStructureHeader{}
	rch := lrit.RiceCompressionHeader{}
	if len(data) >= (16 + 10) {
		data = data[10:]
		if lf, err := lrit.NewLRITFile(0, 0, data, true, 0); err == nil {
			endOfHeaders = int(lf.PrimaryHeader.AllHeaderLength) + 10
			if lf.IsImageFile() {
				if tmp := lf.FindSecondaryHeader(lrit.ImageStructureHeaderType); tmp != nil {
					ish = tmp.(lrit.ImageStructureHeader)
				}

				if tmp := lf.FindSecondaryHeader(lrit.RiceCompressionHeaderType); tmp != nil {
					rch = tmp.(lrit.RiceCompressionHeader)
				}

				if ish != (lrit.ImageStructureHeader{}) && rch != (lrit.RiceCompressionHeader{}) {
					if ish.IsCompressed == 1 {
						needsDecompress = true
					}
				}
			}
		}
	}
	return needsDecompress, endOfHeaders, rch, ish
}

func DecompressSDUBuffer(data []byte, rch lrit.RiceCompressionHeader, ish lrit.ImageStructureHeader) ([]byte, error) {
	var ret []byte
	if decompressed, err := lrit.RiceDecompressBuffer(data, rch, ish); err == nil {
		ret = append(ret, decompressed...)
	} else {
		return ret, fmt.Errorf("Decompression failed with %s", err.Error())
	}
	return ret, nil
}

// TODO: Clean this up
//func SDUNeedsDecompress(data []byte) (bool, int, lrit.RiceCompressionHeader, lrit.ImageStructureHeader) {
//	needsDecompress := false
//	endOfHeaders := 0
//	ish := lrit.ImageStructureHeader{}
//	rch := lrit.RiceCompressionHeader{}
//	if len(data) >= (16 + 10) {
//		data = data[10:]
//		ph := lrit.MakePrimaryHeader(data)
//		if ph.Length <= uint16(len(data)) {
//			data = data[ph.Length:]
//			endOfHeaders = int(ph.AllHeaderLength)
//			//Is an image file
//			if ph.FileType == 0 {
//				lf := lrit.File{Data: data, PrimaryHeader: ph}
//				lf.PopulateSecondaryHeaders()
//
//				tmpish := lf.FindSecondaryHeader(lrit.ImageStructureHeaderType)
//				if tmpish != nil {
//					ish = tmpish.(lrit.ImageStructureHeader)
//				}
//				if ish != (lrit.ImageStructureHeader{}) {
//					tmprch := lf.FindSecondaryHeader(lrit.RiceCompressionHeaderType)
//					if tmprch != nil {
//						rch = tmprch.(lrit.RiceCompressionHeader)
//					}
//					if ish.IsCompressed == 1 {
//						needsDecompress = true
//					}
//				}
//			}
//		}
//	}
//	return needsDecompress, endOfHeaders, rch, ish
//}

func SDUIsImage(data []byte) bool {
	data = data[10:]
	ph := lrit.MakePrimaryHeader(data)
	if ph.FileType == 0 {
		return true
	}
	return false
}

func DecompressSDU(data []byte, packetidx int, numPacketsInFile int, endOfHeaders int, rch lrit.RiceCompressionHeader, ish lrit.ImageStructureHeader) ([]byte, error) {
	var ret []byte
	var decompressbuf []byte
	var savedheaders []byte

	if packetidx > 0 {
		// If this is a continuation sdu, we decompress the whole dang thing
		decompressbuf = data
	} else {
		//If we have an sdu with LRIT headers, those arent compressed, so we save them
		savedheaders = data[:endOfHeaders]
		decompressbuf = data[endOfHeaders:]
	}

	if decompressed, err := lrit.RiceDecompressBuffer(decompressbuf, rch, ish); err == nil {
		// Once decompressed, append all the headers, if we have them
		if len(savedheaders) > 0 {
			ret = savedheaders
		}
		// ...and then replace the compressed bytes with the decompressed bytes
		ret = append(ret, decompressed...)
	} else {
		return ret, fmt.Errorf("Decompression failed with %s", err.Error())
	}
	return ret, nil
}

func GetImageStructureHeaderForSDU(data []byte) (lrit.ImageStructureHeader, error) {
	//Skip the transport header
	data = data[10:]
	if lf, err := lrit.NewLRITFile(0, 0, data, true, 0); err == nil {
		tmpish := lf.FindSecondaryHeader(lrit.ImageStructureHeaderType)
		if tmpish == nil {
			return lrit.ImageStructureHeader{}, fmt.Errorf("Could not find ImageStructureHeader!")
		}
		return tmpish.(lrit.ImageStructureHeader), nil
	} else {
		return lrit.ImageStructureHeader{}, err
	}
}

func GetFillSDUs(data []byte, missingSDUs uint) ([]byte, error) {
	var ret []byte
	//TODO: Fix this; for some reason its not filling in all the lines that we should be able to...
	//if lf, err := lrit.NewLRITFile(0, 0, data, true, 0); err == nil {
	//	if lf.IsImageFile() {
	//		if ish := lf.FindSecondaryHeader(lrit.ImageStructureHeaderType); ish != nil {
	//			for i := uint(0); i < missingSDUs; i++ {
	//				if len(data)-int(lf.PrimaryHeader.AllHeaderLength) == 0 {
	//					fill := make([]byte, ish.(lrit.ImageStructureHeader).NumCols)
	//					ret = append(ret, fill...)
	//				} else {
	//					tmp := data[len(data)-int(ish.(lrit.ImageStructureHeader).NumCols):]
	//					if len(tmp) != int(ish.(lrit.ImageStructureHeader).NumCols) {
	//						log.Errorf("Fill SDU size mismatch! Have: %d Want: %d", len(tmp), ish.(lrit.ImageStructureHeader).NumCols)
	//					}
	//					ret = append(ret, tmp...)
	//				}

	//			}
	//			return ret, nil
	//		} else {
	//			log.Error("Not filling image; no ISH")
	//			return ret, fmt.Errorf("Could not find Image Structure Header: %s", err.Error())
	//		}
	//	} else {
	//		log.Error("Not am image; not filling")
	//		return ret, fmt.Errorf("Missing SDU is not an image; Not filling...")
	//	}
	//} else {
	//	return ret, err
	//}

	//	ret = []byte{}

	//If we can't make an LRIT file (meaning we don't have enough data for all the headers yet
	//Just fill with null bytes (black line)

	if SDUIsImage(data) {
		if ish, err := GetImageStructureHeaderForSDU(data); err == nil {
			for i := uint(0); i < missingSDUs; i++ {
				fill := make([]byte, ish.NumCols)
				ret = append(ret, fill...)
			}
		} else {
			return ret, fmt.Errorf("Could not find Image Structure Header: %s", err.Error())
		}
	} else {
		return ret, fmt.Errorf("Missing SDU is not an image! Can not fill...")
	}

	return ret, nil
}

func (l *TransportLayer) CreateTransportFile(sdus []*packets.MSDU) *packets.TransportFile {
	t := packets.TransportFile{}

	var data []byte

	sort.Slice(sdus, func(a, b int) bool {
		return sdus[a].Header.PacketSequenceCounter < sdus[b].Header.PacketSequenceCounter
	})

	var needsDecompress bool
	var caughtUp bool
	var endOfHeaders int
	var rch lrit.RiceCompressionHeader
	var ish lrit.ImageStructureHeader
	for idx, sdu := range sdus {
		if idx > 0 {
			if diff := packets.CounterDiff(16384, uint32(sdus[idx-1].Header.PacketSequenceCounter), uint32(sdu.Header.PacketSequenceCounter)) - 1; diff > 0 {
				log.Debugf("Missing SDU! Last packet: %d, current packet: %d", sdus[idx-1].Header.PacketSequenceCounter, sdu.Header.PacketSequenceCounter)
				//TODO: Ensure this works!
				if ish != (lrit.ImageStructureHeader{}) {
					for i := uint(0); i < diff; i++ {
						data = append(data, make([]byte, ish.NumCols)...)
					}
				} else {
					//	if fill, err := GetFillSDUs(data, diff); err == nil && l.FillMissingSDUWithNull {
					//		// Add null bytes when missing chunk
					//		data = append(data, fill...)
					//	} else {
					log.Debug("nah")
					//	}
				}
			}
		}

		// Check if we have a compressed image packet
		//TODO: SOomehow this works. I have no idea how or why.
		//	Its possible we're still dropping packets or not decompressing data when we should be,
		//	but this at least semi works.
		//	Next step: figure out what we need to do to concat the lrit files into a single image?
		//	Does it need to combine LRIT files? Or do it at the presentation layer?
		if ish == (lrit.ImageStructureHeader{}) && rch == (lrit.RiceCompressionHeader{}) {
			needsDecompress, endOfHeaders, rch, ish = SDUNeedsDecompress(data)
		}

		// If we have a compressed image packet, decompress each SDU *after* the headers and before the CRC!
		alreadyDecompressedSDU := false
		if needsDecompress {
			if !caughtUp {
				if endOfHeaders < len(data) {
					if d, err := lrit.RiceDecompressBuffer(data[endOfHeaders:], rch, ish); err == nil {
						data = data[:endOfHeaders]
						data = append(data, d...)
					} else {
						//If decompression fails, just insert black lines
						//postHeaderLen := len(data[endOfHeaders:])
						data = data[:endOfHeaders]
						data = append(data, make([]byte, ish.NumCols)...)
					}
				} else if endOfHeaders > len(data) {
					alreadyDecompressedSDU = true
					buf := append(data, sdu.Data...)
					if d, err := lrit.RiceDecompressBuffer(buf[endOfHeaders:], rch, ish); err == nil {
						data = buf[:endOfHeaders]
						data = append(data, d...)
					} else {
						//If decompression fails, just insert black lines
						log.Errorf("RICE decompress failed: %s", err.Error())
						//postHeaderLen := len(buf[endOfHeaders:])
						data = buf[:endOfHeaders]
						data = append(data, make([]byte, ish.NumCols)...)
					}
				}
				caughtUp = true
			}
			if !alreadyDecompressedSDU {
				if d, err := lrit.RiceDecompressBuffer(sdu.Data, rch, ish); err == nil {
					data = append(data, d...)
				} else {
					//If decompression fails, just insert black lines
					log.Errorf("RICE decompress failed: %s", err.Error())
					data = append(data, make([]byte, ish.NumCols)...)
				}
			}
		} else {
			data = append(data, sdu.Data...)
		}
	}

	t.Data = data

	var err error
	t.Header, err = MakeTransportFileHeader(data)
	if err != nil {
		log.Fatal(err)
	} else {
		data = data[10:] // Shift past header
	}

	t.VCDUVersion = sdus[len(sdus)-1].VCDUVersion
	t.VCID = sdus[len(sdus)-1].VCID
	t.Version = sdus[len(sdus)-1].Header.Version
	t.Type = sdus[len(sdus)-1].Header.Type
	t.SecondaryHeaderFlag = sdus[len(sdus)-1].Header.SecondaryHeaderFlag
	t.APID = sdus[len(sdus)-1].Header.APID
	t.PacketLength = sdus[len(sdus)-1].Header.PacketLength
	t.CRCGood = true
	t.Data = data

	return &t
}

func (t *TransportLayer) ProcessVCDU(vcdu *packets.VCDU) {
	if sdus, err := t.Assemblers[vcdu.VCID].ParseMSDUs(vcdu); err != nil {
		log.Error(err)
	} else {
		if !vcdu.IsCorrupt {
			for _, sdu := range sdus {
				if sdu.Header.APID == 2047 || len(sdu.Data) <= 2 {
					//Drop fill packets
					continue
				}

				//Calculate our CRC and check our CRC's
				CRC := (uint16(sdu.Data[len(sdu.Data)-2]) << 8) | uint16(sdu.Data[len(sdu.Data)-1])
				sdu.Data = sdu.Data[:len(sdu.Data)-2]

				calcCRC := packets.CalcCRCBuffer(sdu.Data)
				if calcCRC != CRC {
					log.Debugf("<TRANSPORT> Detected CRC mismatch in SDU for packet. Have: %d, want: %d", calcCRC, CRC)
					if !t.ContinueOnCRCFailure {
						//If we don't have a valid CRC, drop this SDU
						//This affects ALL SDU's! So when we check for Image CRC's and want to let them through, we can't. Also this now makes me question WHY we get garbage sometimes....
						continue
					}
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
					t.ClearIncompletePacketBuffer(vcdu.VCID, sdu.Header.APID)
					t.InsertIncompletePacket(vcdu.VCID, &sdu)
				case 2:
					//End packet
					t.InsertIncompletePacket(vcdu.VCID, &sdu)
					tppdu := t.CreateTransportFile(t.IncompletePackets[vcdu.VCID][sdu.Header.APID])
					//Clear out the buffer of incomplete packets
					t.ClearIncompletePacketBuffer(vcdu.VCID, sdu.Header.APID)
					if tppdu != nil {
						*t.TransportOutput <- tppdu
					}
				case 3:
					//Self contained packet
					tppdu := t.CreateTransportFile([]*packets.MSDU{&sdu})
					if tppdu != nil {
						*t.TransportOutput <- tppdu
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
	return t.TransportOutput
}

func (t *TransportLayer) Reset() {
}

func (t *TransportLayer) Flush() {
}

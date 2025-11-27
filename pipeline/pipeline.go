package pipeline

import (
	"fmt"

	"github.com/jrwynneiii/ccsds_tools"
	"github.com/jrwynneiii/ccsds_tools/layers/datalink"
	"github.com/jrwynneiii/ccsds_tools/layers/physical"
	"github.com/jrwynneiii/ccsds_tools/layers/session"
	"github.com/jrwynneiii/ccsds_tools/layers/transport"
	"github.com/jrwynneiii/ccsds_tools/lrit"
	"github.com/jrwynneiii/ccsds_tools/packets"
	"github.com/jrwynneiii/ccsds_tools/types"
	"github.com/knadh/koanf/v2"
)

type Pipeline struct {
	Layers              []ccsds_tools.Layer
	SampleRate          float32
	BufferSize          uint
	configFile          *koanf.Koanf
	options             map[string]any
	NumLayersRegistered int
}

func New(configFile *koanf.Koanf) *Pipeline {
	srate := configFile.Float64("radio.sample_rate")
	bufsize := uint(configFile.Int("xrit.chunk_size"))
	return &Pipeline{
		SampleRate: float32(srate),
		BufferSize: bufsize,
		Layers:     make([]ccsds_tools.Layer, 6),
		configFile: configFile,
	}
}

func NewWithOptionsMap(options map[string]any) *Pipeline {
	srate := options["radio.sample_rate"].(float64)
	bufsize := uint(options["xrit.chunk_size"].(int))
	return &Pipeline{
		SampleRate: float32(srate),
		BufferSize: bufsize,
		Layers:     make([]ccsds_tools.Layer, 6),
		options:    options,
	}
}

// TODO: Add a RegisterWithOptions() method that takes a map of options, where the map == map[LayerType]OptionStruct,
func (p *Pipeline) Register(id ccsds_tools.LayerType) {
	switch id {
	case ccsds_tools.PhysicalLayer:
		input := make(chan []complex64, p.BufferSize)
		output := make(chan byte, p.BufferSize)

		var xritConf types.XRITConf
		var agcConf types.AGCConf
		var clockConf types.ClockRecoveryConf
		if p.configFile != nil {
			xritConf = types.XRITConf{
				SymbolRate:             p.configFile.Float64("xrit.symbol_rate"),
				RRCAlpha:               p.configFile.Float64("xrit.rrc_alpha"),
				RRCTaps:                p.configFile.Int("xrit.rrc_taps"),
				LowPassTransitionWidth: p.configFile.Float64("xrit.lowpass_transition_width"),
				PLLAlpha:               float32(p.configFile.Float64("xrit.pll_alpha")),
				Decimation:             p.configFile.Int("xrit.decimation_factor"),
				ChunkSize:              uint(p.configFile.Int("xrit.chunk_size")),
				DoFFT:                  p.configFile.Bool("xrit.do_fft"),
			}

			agcConf = types.AGCConf{
				Rate:      float32(p.configFile.Float64("agc.rate")),
				Reference: float32(p.configFile.Float64("agc.reference")),
				Gain:      float32(p.configFile.Float64("agc.gain")),
				MaxGain:   float32(p.configFile.Float64("agc.max_gain")),
			}

			clockConf = types.ClockRecoveryConf{
				Mu:         float32(p.configFile.Float64("clockrecovery.mu")),
				Alpha:      float32(p.configFile.Float64("clockrecovery.alpha")),
				OmegaLimit: float32(p.configFile.Float64("clockrecovery.omega_limit")),
			}
		} else {
			xritConf = types.XRITConf{
				SymbolRate:             p.options["xrit.symbol_rate"].(float64),
				RRCAlpha:               p.options["xrit.rrc_alpha"].(float64),
				RRCTaps:                p.options["xrit.rrc_taps"].(int),
				LowPassTransitionWidth: p.options["xrit.lowpass_transition_width"].(float64),
				PLLAlpha:               float32(p.options["xrit.pll_alpha"].(float64)),
				Decimation:             p.options["xrit.decimation_factor"].(int),
				ChunkSize:              uint(p.options["xrit.chunk_size"].(int)),
				DoFFT:                  p.options["xrit.do_fft"].(bool),
			}

			agcConf = types.AGCConf{
				Rate:      float32(p.options["agc.rate"].(float64)),
				Reference: float32(p.options["agc.reference"].(float64)),
				Gain:      float32(p.options["agc.gain"].(float64)),
				MaxGain:   float32(p.options["agc.max_gain"].(float64)),
			}

			clockConf = types.ClockRecoveryConf{
				Mu:         float32(p.options["clockrecovery.mu"].(float64)),
				Alpha:      float32(p.options["clockrecovery.alpha"].(float64)),
				OmegaLimit: float32(p.options["clockrecovery.omega_limit"].(float64)),
			}
		}

		layer := physical.New(p.SampleRate, p.BufferSize, xritConf, agcConf, clockConf, &input, &output)
		p.Layers[id] = layer
		p.NumLayersRegistered++
	case ccsds_tools.DataLinkLayer:
		output := make(chan []byte, p.BufferSize)

		var vitConf types.ViterbiConf
		var xritConf types.XRITFrameConf
		if p.configFile != nil {
			vitConf = types.ViterbiConf{
				MaxErrors: p.configFile.Int("viterbi.max_errors"),
			}
			xritConf = types.XRITFrameConf{
				FrameSize:     p.configFile.Int("xritframe.frame_size"),
				LastFrameSize: p.configFile.Int("xritframe.last_frame_size"),
			}
		} else {
			vitConf = types.ViterbiConf{
				MaxErrors: p.options["viterbi.max_errors"].(int),
			}
			xritConf = types.XRITFrameConf{
				FrameSize:     p.options["xritframe.frame_size"].(int),
				LastFrameSize: p.options["xritframe.last_frame_size"].(int),
			}
		}

		layer := datalink.New(p.BufferSize, vitConf, xritConf, p.Layers[id-1].GetOutput().(*chan byte), &output)
		p.Layers[id] = layer
		p.NumLayersRegistered++
	case ccsds_tools.TransportLayer:
		output := make(chan *packets.TransportFile, p.BufferSize)
		layer := transport.New(p.Layers[id-1].GetOutput().(*chan []byte), &output)
		p.Layers[id] = layer
		p.NumLayersRegistered++
	case ccsds_tools.SessionLayer:
		output := make(chan *lrit.File, p.BufferSize)
		layer := session.New(p.Layers[id-1].GetOutput().(*chan *packets.TransportFile), &output)
		p.Layers[id] = layer
		p.NumLayersRegistered++
	case ccsds_tools.PresentationLayer:
	case ccsds_tools.ApplicationLayer:
	default:
		panic(fmt.Errorf("Could not add layer id %d to pipeline", id))
	}
}

func (p *Pipeline) Start() {
	for i := 0; i < p.NumLayersRegistered; i++ {
		go p.Layers[i].Start()
	}
}

func (p *Pipeline) Destroy() {
	for i := p.NumLayersRegistered - 1; i > 0; i-- {
		p.Layers[i].Destroy()
	}
}

func (p *Pipeline) Flush() {
	for i := 0; i < p.NumLayersRegistered; i++ {
		p.Layers[i].Flush()
	}
}

func (p *Pipeline) Reset() {
	p.Flush()
	for i := 0; i < p.NumLayersRegistered; i++ {
		p.Layers[i].Reset()
	}
}

package pipeline

import (
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/jrwynneiii/ccsds_tools"
	"github.com/jrwynneiii/ccsds_tools/layers/datalink"
	"github.com/jrwynneiii/ccsds_tools/layers/physical"
	"github.com/jrwynneiii/ccsds_tools/types"
	"github.com/knadh/koanf/v2"
)

type Pipeline struct {
	Layers              []ccsds_tools.Layer
	SampleRate          float32
	BufferSize          uint
	configFile          *koanf.Koanf
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

func (p *Pipeline) Register(id ccsds_tools.LayerType) {
	switch id {
	case ccsds_tools.PhysicalLayer:
		input := make(chan []complex64, p.BufferSize)
		output := make(chan byte, p.BufferSize)

		xritConf := types.XRITConf{
			SymbolRate:             p.configFile.Float64("xrit.symbol_rate"),
			RRCAlpha:               p.configFile.Float64("xrit.rrc_alpha"),
			RRCTaps:                p.configFile.Int("xrit.rrc_taps"),
			LowPassTransitionWidth: p.configFile.Float64("xrit.lowpass_transition_width"),
			PLLAlpha:               float32(p.configFile.Float64("xrit.pll_alpha")),
			Decimation:             p.configFile.Int("xrit.decimation_factor"),
			ChunkSize:              uint(p.configFile.Int("xrit.chunk_size")),
			DoFFT:                  p.configFile.Bool("xrit.do_fft"),
		}

		agcConf := types.AGCConf{
			Rate:      float32(p.configFile.Float64("agc.rate")),
			Reference: float32(p.configFile.Float64("agc.reference")),
			Gain:      float32(p.configFile.Float64("agc.gain")),
			MaxGain:   float32(p.configFile.Float64("agc.max_gain")),
		}

		clockConf := types.ClockRecoveryConf{
			Mu:         float32(p.configFile.Float64("clockrecovery.mu")),
			Alpha:      float32(p.configFile.Float64("clockrecovery.alpha")),
			OmegaLimit: float32(p.configFile.Float64("clockrecovery.omega_limit")),
		}

		layer := physical.New(p.SampleRate, p.BufferSize, xritConf, agcConf, clockConf, &input, &output)
		p.Layers[id] = layer
		p.NumLayersRegistered++
	case ccsds_tools.DataLinkLayer:
		output := make(chan []byte, p.BufferSize)

		vitConf := types.ViterbiConf{
			MaxErrors: p.configFile.Int("viterbi.max_errors"),
		}
		xritConf := types.XRITFrameConf{
			FrameSize:     p.configFile.Int("xritframe.frame_size"),
			LastFrameSize: p.configFile.Int("xritframe.last_frame_size"),
		}

		layer := datalink.New(p.BufferSize, vitConf, xritConf, p.Layers[id-1].GetOutput().(*chan byte), &output)
		p.Layers[id] = layer
		p.NumLayersRegistered++
	case ccsds_tools.TransportLayer:
	case ccsds_tools.SessionLayer:
	case ccsds_tools.PresentationLayer:
	case ccsds_tools.ApplicationLayer:
	default:
		panic(fmt.Errorf("Could not add layer id %d to pipeline", id))
	}
}

func (p *Pipeline) Start() {
	for id, layer := range p.Layers {
		log.Infof("Starting layer: %d", id)
		go layer.Start()
	}
}

func (p *Pipeline) Destroy() {
	for i := p.NumLayersRegistered - 1; i > 0; i-- {
		p.Layers[i].Destroy()
	}
}

func (p *Pipeline) Pause() {
}

func (p *Pipeline) Flush() {
}

func (p *Pipeline) FlushLayer(id ccsds_tools.LayerType) {
}

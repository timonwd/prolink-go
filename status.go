package prolink

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"strconv"
)

// Status flag bitmasks
const (
	statusFlagOnAir   byte = 1 << 3
	statusFlagSync    byte = 1 << 4
	statusFlagMaster  byte = 1 << 5
	statusFlagPlaying byte = 1 << 6
)

// Play state flags
const (
	PlayStateEmpty     PlayState = 0x00
	PlayStateLoading   PlayState = 0x02
	PlayStatePlaying   PlayState = 0x03
	PlayStateLooping   PlayState = 0x04
	PlayStatePaused    PlayState = 0x05
	PlayStateCued      PlayState = 0x06
	PlayStateCuing     PlayState = 0x07
	PlayStateSearching PlayState = 0x09
	PlayStateSpunDown  PlayState = 0x0e
	PlayStateEnded     PlayState = 0x11
)

// Labels associated to the PlayState flags
var playStateLabels = map[PlayState]string{
	PlayStateEmpty:     "empty",
	PlayStateLoading:   "loading",
	PlayStatePlaying:   "playing",
	PlayStateLooping:   "looping",
	PlayStatePaused:    "paused",
	PlayStateCued:      "cued",
	PlayStateCuing:     "cuing",
	PlayStateSearching: "searching",
	PlayStateSpunDown:  "spun_down",
	PlayStateEnded:     "ended",
}

// PlayState represents the play state of the CDJ.
type PlayState byte

// String returns the string representation of the play state.
func (s PlayState) String() string {
	return playStateLabels[s]
}

// Track load slot flags
const (
	TrackSlotEmpty TrackSlot = 0x00
	TrackSlotCD    TrackSlot = 0x01
	TrackSlotSD    TrackSlot = 0x02
	TrackSlotUSB   TrackSlot = 0x03
	TrackSlotRB    TrackSlot = 0x04
)

// Labels associated to the track load slot flags
var trackSlotLabels = map[TrackSlot]string{
	TrackSlotEmpty: "empty",
	TrackSlotCD:    "cd",
	TrackSlotSD:    "sd",
	TrackSlotUSB:   "usb",
	TrackSlotRB:    "rekordbox",
}

// TrackSlot label to TrackSlot mapping
var labelsTrackSlot = map[string]TrackSlot{
	"empty":     TrackSlotEmpty,
	"cd":        TrackSlotCD,
	"sd":        TrackSlotSD,
	"usb":       TrackSlotUSB,
	"rekordbox": TrackSlotRB,
}

// TrackSlot represents the slot that a track is loaded from on the CDJ.
type TrackSlot byte

// String returns the string representation of the track slot.
func (s TrackSlot) String() string {
	return trackSlotLabels[s]
}

// Track type flags
const (
	TrackTypeNone       TrackType = 0x00
	TrackTypeRB         TrackType = 0x01
	TrackTypeUnanalyzed TrackType = 0x02
	TrackTypeAudioCD    TrackType = 0x05
)

// Labels associated to the track type flags
var trackTypeLabels = map[TrackType]string{
	TrackTypeNone:       "none",
	TrackTypeRB:         "rekordbox",
	TrackTypeUnanalyzed: "unanalyzed",
	TrackTypeAudioCD:    "audio_cd",
}

// TrackType label to TrackType mapping
var labelsTrackType = map[string]TrackType{
	"none":       TrackTypeNone,
	"rekordbox":  TrackTypeRB,
	"unanalyzed": TrackTypeUnanalyzed,
	"audio_cd":   TrackTypeAudioCD,
}

// TrackType represents the type of track.
type TrackType byte

// String returns the string representation of the track type.
func (t TrackType) String() string {
	return trackTypeLabels[t]
}

// CDJStatus represents various details about the current state of the CDJ.
type CDJStatus struct {
	PlayerID       DeviceID
	TrackID        uint32
	TrackDevice    DeviceID
	TrackSlot      TrackSlot
	TrackType      TrackType
	PlayState      PlayState
	IsOnAir        bool
	IsSync         bool
	IsMaster       bool
	TrackBPM       float32
	EffectivePitch float32
	SliderPitch    float32
	BeatInMeasure  uint32
	BeatsUntilCue  uint16
	Beat           uint32
	PacketNum      uint32
}

// TrackKey constructs a track query object from the CDJStatus. If no track
// is currently provided in the CDJStatus nil will be returned.
func (s *CDJStatus) TrackKey() *TrackKey {
	if s.TrackID == 0 {
		return nil
	}

	return &TrackKey{
		DeviceID: s.TrackDevice,
		Slot:     s.TrackSlot,
		Type:     s.TrackType,
		TrackID:  s.TrackID,
	}
}

func (s *CDJStatus) String() string {
	statusText := `Status of Device %d (packet %d)
  Track  %-9s [from device %d, slot %s, type %s]
  BPM    %-9s [pitch %2.2f%%, effective pitch %2.2f%%]
  Beat   %-9s [%d/4, %d beats to cue]
  Status %-9s [synced: %t, onair: %t, master: %t]`

	return fmt.Sprintf(statusText,
		s.PlayerID,
		s.PacketNum,
		strconv.Itoa(int(s.TrackID)),
		s.TrackDevice,
		trackSlotLabels[s.TrackSlot],
		trackTypeLabels[s.TrackType],
		fmt.Sprintf("%2.2f", s.TrackBPM),
		s.SliderPitch,
		s.EffectivePitch,
		strconv.Itoa(int(s.Beat)),
		s.BeatInMeasure,
		s.BeatsUntilCue,
		playStateLabels[s.PlayState],
		s.IsSync,
		s.IsOnAir,
		s.IsMaster,
	)
}

func packetToStatus(p []byte) (*CDJStatus, error) {
	if !bytes.HasPrefix(p, prolinkHeader) {
		return nil, fmt.Errorf("CDJ status packet does not start with the expected header")
	}

	if len(p) < 0xFF {
		return nil, nil
	}

	status := &CDJStatus{
		PlayerID:       DeviceID(p[0x21]),
		TrackID:        be.Uint32(p[0x2C : 0x2C+4]),
		TrackDevice:    DeviceID(p[0x28]),
		TrackSlot:      TrackSlot(p[0x29]),
		TrackType:      TrackType(p[0x2a]),
		PlayState:      PlayState(p[0x7B]),
		IsOnAir:        p[0x89]&statusFlagOnAir != 0,
		IsSync:         p[0x89]&statusFlagSync != 0,
		IsMaster:       p[0x89]&statusFlagMaster != 0,
		TrackBPM:       calcBPM(p[0x92 : 0x92+2]),
		SliderPitch:    calcPitch(p[0x8D : 0x8D+3]),
		EffectivePitch: calcPitch(p[0x99 : 0x99+3]),
		BeatInMeasure:  uint32(p[0xA6]),
		BeatsUntilCue:  be.Uint16(p[0xA4 : 0xA4+2]),
		Beat:           be.Uint32(p[0xA0 : 0xA0+4]),
		PacketNum:      be.Uint32(p[0xC8 : 0xC8+4]),
	}

	return status, nil
}

// calcPitch converts a uint24 byte value into a flaot32 pitch.
//
// The pitch information ranges from 0x000000 (meaning -100%, complete stop) to
// 0x200000 (+100%).
func calcPitch(p []byte) float32 {
	p = append([]byte{0x00}, p[:]...)

	v := float32(be.Uint32(p))
	d := float32(0x100000)

	return (v - d) / d * 100
}

// calcBPM converts a uint16 byte value into a float32 bpm.
func calcBPM(p []byte) float32 {
	val := be.Uint16(p)

	if val == math.MaxUint16 {
		return 0
	}

	return float32(val) / 100
}

// A StatusHandler responds to status updates on a CDJ.
type StatusHandler interface {
	OnStatusUpdate(*CDJStatus)
}

// The StatusHandlerFunc is an adapter to allow a function to be used as a
// StatusHandler.
type StatusHandlerFunc func(*CDJStatus)

// OnStatusUpdate implements StatusHandler.
func (f StatusHandlerFunc) OnStatusUpdate(s *CDJStatus) { f(s) }

// CDJStatusMonitor provides an interface for watching for status updates to
// CDJ devices on the PRO DJ LINK network.
type CDJStatusMonitor struct {
	handlers []StatusHandler
}

// AddStatusHandler registers a StatusHandler to be called when any CDJ on the
// PRO DJ LINK network reports its status.
func (sm *CDJStatusMonitor) AddStatusHandler(h StatusHandler) {
	sm.handlers = append(sm.handlers, h)
}

// activate triggers the CDJStatusMonitor to begin listening for status packets
// given a UDP connection to listen on.
func (sm *CDJStatusMonitor) activate(listenConn io.Reader) {
	packet := make([]byte, 512)

	statusUpdateHandler := func() {
		n, err := listenConn.Read(packet)
		if err != nil || n == 0 {
			return
		}

		status, err := packetToStatus(packet[:n])
		if err != nil {
			return
		}

		if status == nil {
			return
		}

		for _, h := range sm.handlers {
			go h.OnStatusUpdate(status)
		}
	}

	go func() {
		for {
			statusUpdateHandler()
		}
	}()
}

func newCDJStatusMonitor() *CDJStatusMonitor {
	return &CDJStatusMonitor{handlers: []StatusHandler{}}
}

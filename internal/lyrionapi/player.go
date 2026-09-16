package lyrionapi

import (
	"fmt"
	"strconv"
	"strings"
)

type LyrionPlayer struct {
	id     string
	name   string
	model  string
	server *LyrionServer
}

func (server *LyrionServer) GetPlayerCount() (int, error) {
	response, err := server.Query("player count ?")
	if err != nil {
		return -1, err
	}

	// expected response is 14 characters in
	count, err := strconv.ParseInt(response[13:], 10, 0)
	if err != nil {
		return -1, err
	}

	return int(count), nil
}

func (server *LyrionServer) GetPlayer(index int) (*LyrionPlayer, error) {
	request := fmt.Sprintf("player id %v ?", index)
	id, err := server.Query(request)
	if err != nil {
		return nil, err
	}
	id = id[len(request)-1:]

	request = fmt.Sprintf("%v name ?", id)
	name, err := server.Query(request)
	if err != nil {
		return nil, err
	}
	name = name[len(request)-1:]

	request = fmt.Sprintf("player model %v ?", id)
	model, err := server.Query(request)
	if err != nil {
		return nil, err
	}
	model = model[len(request)-1:]

	return &LyrionPlayer{
		id:     id,
		name:   name,
		model:  model,
		server: server,
	}, nil
}

func (player *LyrionPlayer) UUID() (string, error) {
	return player.queryField(fmt.Sprintf("player uuid %v ?", player.id))
}

func (player *LyrionPlayer) IP() (string, error) {
	return player.queryField(fmt.Sprintf("player ip %v ?", player.id))
}

func (player *LyrionPlayer) IsPlayer() (bool, error) {
	return player.queryBoolField(fmt.Sprintf("player isplayer %v ?", player.id))
}

func (player *LyrionPlayer) DisplayType() (string, error) {
	return player.queryField(fmt.Sprintf("player displaytype %v ?", player.id))
}

func (player *LyrionPlayer) CanPowerOff() (bool, error) {
	return player.queryBoolField(fmt.Sprintf("player canpoweroff %v ?", player.id))
}

func (player *LyrionPlayer) SignalStrength() (int, error) {
	return player.queryIntField(fmt.Sprintf("%v signalstrength ?", player.id))
}

func (player *LyrionPlayer) Connected() (bool, error) {
	return player.queryBoolField(fmt.Sprintf("%v connected ?", player.id))
}

func (player *LyrionPlayer) SetName(name string) error {
	_, err := player.server.Query(fmt.Sprintf("%v name %v", player.id, encodeArg(name)))
	return err
}

// Sleep returns the number of seconds remaining until the player automatically powers off.
func (player *LyrionPlayer) Sleep() (float64, error) {
	return player.queryFloatField(fmt.Sprintf("%v sleep ?", player.id))
}

func (player *LyrionPlayer) SetSleep(seconds int) error {
	_, err := player.server.Query(fmt.Sprintf("%v sleep %v", player.id, seconds))
	return err
}

// SyncStatus returns the IDs of the players this player is synced with, or
// "-" if it isn't synced with anything.
func (player *LyrionPlayer) SyncStatus() (string, error) {
	return player.queryField(fmt.Sprintf("%v sync ?", player.id))
}

// Sync synchronizes this player with target (a player index or player ID).
func (player *LyrionPlayer) Sync(target string) error {
	_, err := player.server.Query(fmt.Sprintf("%v sync %v", player.id, target))
	return err
}

func (player *LyrionPlayer) Unsync() error {
	_, err := player.server.Query(fmt.Sprintf("%v sync -", player.id))
	return err
}

func (player *LyrionPlayer) Power() (bool, error) {
	return player.queryBoolField(fmt.Sprintf("%v power ?", player.id))
}

func (player *LyrionPlayer) SetPower(on bool) error {
	_, err := player.server.Query(fmt.Sprintf("%v power %v", player.id, boolToInt(on)))
	return err
}

func (player *LyrionPlayer) TogglePower() error {
	_, err := player.server.Query(fmt.Sprintf("%v power", player.id))
	return err
}

func (player *LyrionPlayer) Volume() (int, error) {
	return player.queryIntField(fmt.Sprintf("%v mixer volume ?", player.id))
}

func (player *LyrionPlayer) SetVolume(volume int) error {
	_, err := player.server.Query(fmt.Sprintf("%v mixer volume %v", player.id, volume))
	return err
}

func (player *LyrionPlayer) Muting() (bool, error) {
	return player.queryBoolField(fmt.Sprintf("%v mixer muting ?", player.id))
}

func (player *LyrionPlayer) SetMuting(muted bool) error {
	_, err := player.server.Query(fmt.Sprintf("%v mixer muting %v", player.id, boolToInt(muted)))
	return err
}

// Bass, Treble and Pitch are only supported on SliMP3/SB1-generation hardware.
func (player *LyrionPlayer) Bass() (int, error) {
	return player.queryIntField(fmt.Sprintf("%v mixer bass ?", player.id))
}

func (player *LyrionPlayer) SetBass(bass int) error {
	_, err := player.server.Query(fmt.Sprintf("%v mixer bass %v", player.id, bass))
	return err
}

func (player *LyrionPlayer) Treble() (int, error) {
	return player.queryIntField(fmt.Sprintf("%v mixer treble ?", player.id))
}

func (player *LyrionPlayer) SetTreble(treble int) error {
	_, err := player.server.Query(fmt.Sprintf("%v mixer treble %v", player.id, treble))
	return err
}

func (player *LyrionPlayer) Pitch() (int, error) {
	return player.queryIntField(fmt.Sprintf("%v mixer pitch ?", player.id))
}

func (player *LyrionPlayer) SetPitch(pitch int) error {
	_, err := player.server.Query(fmt.Sprintf("%v mixer pitch %v", player.id, pitch))
	return err
}

func (player *LyrionPlayer) PlayerPref(name string) (string, error) {
	return player.queryField(fmt.Sprintf("%v playerpref %v ?", player.id, name))
}

func (player *LyrionPlayer) SetPlayerPref(name, value string) error {
	_, err := player.server.Query(fmt.Sprintf("%v playerpref %v %v", player.id, name, encodeArg(value)))
	return err
}

func (player *LyrionPlayer) ValidatePlayerPref(name, value string) (bool, error) {
	response, err := player.server.Query(fmt.Sprintf("%v playerpref validate %v %v", player.id, name, encodeArg(value)))
	if err != nil {
		return false, err
	}
	return strings.Contains(response, "valid:1"), nil
}

type ShowOpts struct {
	Line1      string
	Line2      string
	Duration   string
	Brightness string
	Font       string
	Centered   bool
	Screen     string
}

// Show displays a message on the player's physical display.
func (player *LyrionPlayer) Show(opts ShowOpts) error {
	var args []string
	if opts.Line1 != "" {
		args = append(args, "line1:"+encodeArg(opts.Line1))
	}
	if opts.Line2 != "" {
		args = append(args, "line2:"+encodeArg(opts.Line2))
	}
	if opts.Duration != "" {
		args = append(args, "duration:"+opts.Duration)
	}
	if opts.Brightness != "" {
		args = append(args, "brightness:"+opts.Brightness)
	}
	if opts.Font != "" {
		args = append(args, "font:"+opts.Font)
	}
	if opts.Centered {
		args = append(args, "centered:1")
	}
	if opts.Screen != "" {
		args = append(args, "screen:"+opts.Screen)
	}

	request := fmt.Sprintf("%v show %v", player.id, strings.Join(args, " "))
	_, err := player.server.Query(request)
	return err
}

// Display shows line1/line2 on the player's physical display for duration seconds.
func (player *LyrionPlayer) Display(line1, line2 string, duration int) error {
	request := fmt.Sprintf("%v display %v %v %v", player.id, encodeArg(line1), encodeArg(line2), duration)
	_, err := player.server.Query(request)
	return err
}

func (player *LyrionPlayer) LinesPerScreen() (int, error) {
	return player.queryIntField(fmt.Sprintf("%v linesperscreen ?", player.id))
}

// Button simulates a button press (codes are defined in the server's Default.map file).
func (player *LyrionPlayer) Button(code string) error {
	_, err := player.server.Query(fmt.Sprintf("%v button %v", player.id, code))
	return err
}

// IR simulates receiving an IR code at the given (fractional-second) time.
func (player *LyrionPlayer) IR(code string, time float64) error {
	_, err := player.server.Query(fmt.Sprintf("%v ir %v %v", player.id, code, time))
	return err
}

func (player *LyrionPlayer) IREnable() (bool, error) {
	return player.queryBoolField(fmt.Sprintf("%v irenable ?", player.id))
}

func (player *LyrionPlayer) SetIREnable(enabled bool) error {
	_, err := player.server.Query(fmt.Sprintf("%v irenable %v", player.id, boolToInt(enabled)))
	return err
}

// Connect instructs an SB2+ player to connect to a different server address.
func (player *LyrionPlayer) Connect(ip string) error {
	_, err := player.server.Query(fmt.Sprintf("%v connect %v", player.id, ip))
	return err
}

// Forget removes this player from the server's client database.
func (player *LyrionPlayer) Forget() error {
	_, err := player.server.Query(fmt.Sprintf("%v client forget", player.id))
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

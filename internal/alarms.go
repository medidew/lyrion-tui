package lyrionapi

import (
	"fmt"
	"strconv"
	"strings"
)

type Alarm struct {
	ID          string
	DOW         []int
	Enabled     bool
	Repeat      bool
	ShuffleMode int
	Time        string
	Volume      int
	URL         string
}

type AlarmPlaylist struct {
	Title     string
	Category  string
	URL       string
	Singleton bool
}

func alarmFromRecord(record map[string]string) Alarm {
	return Alarm{
		ID:          tagString(record, "id"),
		DOW:         parseDOW(tagString(record, "dow")),
		Enabled:     tagBool(record, "enabled"),
		Repeat:      tagBool(record, "repeat"),
		ShuffleMode: tagInt(record, "shufflemode"),
		Time:        tagString(record, "time"),
		Volume:      tagInt(record, "volume"),
		URL:         tagString(record, "url"),
	}
}

func parseDOW(raw string) []int {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	days := make([]int, 0, len(parts))
	for _, part := range parts {
		if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			days = append(days, n)
		}
	}
	return days
}

// AlarmOpts fields are all optional (empty string = omit from the command);
// Time and, for AlarmAdd, at least one of the others are required by LMS itself.
type AlarmOpts struct {
	Time    string // seconds from midnight
	DOW     string // comma-separated days of week, 0-6
	DOWAdd  string // single day to add
	DOWDel  string // single day to remove
	Enabled string // "0" or "1"
	Repeat  string // "0" or "1"
	Volume  string
	URL     string
}

func (opts AlarmOpts) args() []string {
	var args []string
	if opts.Time != "" {
		args = append(args, "time:"+opts.Time)
	}
	if opts.DOW != "" {
		args = append(args, "dow:"+opts.DOW)
	}
	if opts.DOWAdd != "" {
		args = append(args, "dowAdd:"+opts.DOWAdd)
	}
	if opts.DOWDel != "" {
		args = append(args, "dowDel:"+opts.DOWDel)
	}
	if opts.Enabled != "" {
		args = append(args, "enabled:"+opts.Enabled)
	}
	if opts.Repeat != "" {
		args = append(args, "repeat:"+opts.Repeat)
	}
	if opts.Volume != "" {
		args = append(args, "volume:"+opts.Volume)
	}
	if opts.URL != "" {
		args = append(args, "url:"+encodeArg(opts.URL))
	}
	return args
}

// AlarmAdd creates a new alarm and returns its ID.
func (player *LyrionPlayer) AlarmAdd(opts AlarmOpts) (string, error) {
	request := fmt.Sprintf("%v alarm add %v", player.id, strings.Join(opts.args(), " "))
	meta, _, err := player.server.queryTagged(request, map[string]bool{"id": true})
	if err != nil {
		return "", err
	}
	if meta["id"] == "" {
		return "", fmt.Errorf("lyrionapi: alarm add did not return an id")
	}
	return meta["id"], nil
}

func (player *LyrionPlayer) AlarmUpdate(id string, opts AlarmOpts) error {
	args := append([]string{"id:" + id}, opts.args()...)
	request := fmt.Sprintf("%v alarm update %v", player.id, strings.Join(args, " "))
	_, err := player.server.Query(request)
	return err
}

func (player *LyrionPlayer) AlarmDelete(id string) error {
	_, err := player.server.Query(fmt.Sprintf("%v alarm delete id:%v", player.id, id))
	return err
}

func (player *LyrionPlayer) AlarmEnableAll() error {
	_, err := player.server.Query(fmt.Sprintf("%v alarm enableall", player.id))
	return err
}

func (player *LyrionPlayer) AlarmDisableAll() error {
	_, err := player.server.Query(fmt.Sprintf("%v alarm disableall", player.id))
	return err
}

func (player *LyrionPlayer) AlarmDefaultVolume(volume int) error {
	_, err := player.server.Query(fmt.Sprintf("%v alarm defaultvolume volume:%v", player.id, volume))
	return err
}

// AlarmPlaylists lists the playlists/sources available for use as an alarm's
// wake-up sound.
func (server *LyrionServer) AlarmPlaylists() ([]AlarmPlaylist, error) {
	_, items, err := server.queryTagged("alarm playlists", map[string]bool{"count": true})
	if err != nil {
		return nil, err
	}

	playlists := make([]AlarmPlaylist, len(items))
	for i, item := range items {
		playlists[i] = AlarmPlaylist{
			Title:     tagString(item, "title"),
			Category:  tagString(item, "category"),
			URL:       tagString(item, "url"),
			Singleton: tagBool(item, "singleton"),
		}
	}
	return playlists, nil
}

type AlarmQueryOpts struct {
	DOW    string // filter by day of week, 0-6
	Filter string // "all" or "enabled"
}

func (opts AlarmQueryOpts) args() []string {
	var args []string
	if opts.DOW != "" {
		args = append(args, "dow:"+opts.DOW)
	}
	if opts.Filter != "" {
		args = append(args, "filter:"+opts.Filter)
	}
	return args
}

func (player *LyrionPlayer) Alarms(from, to int, opts AlarmQueryOpts) ([]Alarm, error) {
	command := player.id + " " + listCommand("alarms", from, to, opts.args())
	_, items, err := player.server.queryTagged(command, map[string]bool{"count": true, "fade": true})
	if err != nil {
		return nil, err
	}

	alarms := make([]Alarm, len(items))
	for i, item := range items {
		alarms[i] = alarmFromRecord(item)
	}
	return alarms, nil
}

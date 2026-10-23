package stats

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// Days are counted in Turkmenistan time, which has no DST.
const tmOffset = 5 * time.Hour

var reportTypes = []string{"track", "album", "playlist", "artist", "video", "video_playlist", "unknown"}

type dayTotals struct {
	views    map[string]int64
	notFound int64
	failed   int64
	bots     int64
	clicks   map[string]int64
}

func newDay() *dayTotals {
	return &dayTotals{views: map[string]int64{}, clicks: map[string]int64{}}
}

// Report prints a per-day table for the last days days, newest first.
func Report(w io.Writer, path string, days int, now time.Time) error {
	if days < 1 {
		days = 1
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("stats file %s not found: nothing has been recorded yet", path)
		}
		return err
	}
	defer f.Close()

	today := now.UTC().Add(tmOffset).Format("2006-01-02")
	first := now.UTC().Add(tmOffset).AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	byDay := map[string]*dayTotals{}
	bots := map[string]int64{}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var l line
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		h, err := time.Parse(hourLayout, l.Hour)
		if err != nil {
			continue
		}
		day := h.Add(tmOffset).Format("2006-01-02")
		if day < first || day > today {
			continue
		}
		d := byDay[day]
		if d == nil {
			d = newDay()
			byDay[day] = d
		}
		for k, n := range l.View {
			d.views[strings.SplitN(k, ".", 2)[0]] += n
		}
		d.notFound += sum(l.NotFound)
		d.failed += sum(l.Failed)
		for k, n := range l.BotView {
			d.bots += n
			bots[k] += n
		}
		for k, n := range l.Click {
			d.clicks[strings.SplitN(k, ".", 2)[0]] += n
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "date\tviews\ttrack\talbum\tplaylist\tartist\tvideo\tvideo_pl\tunknown\tapp_store\tgoogle_play\topen_app\tnot_found%\tfailed%\tbots")
	for i := 0; i < days; i++ {
		day := now.UTC().Add(tmOffset).AddDate(0, 0, -i).Format("2006-01-02")
		d := byDay[day]
		if d == nil {
			d = newDay()
		}
		var views int64
		for _, n := range d.views {
			views += n
		}
		opens := views + d.notFound + d.failed
		fmt.Fprintf(tw, "%s\t%d", day, views)
		for _, t := range reportTypes {
			fmt.Fprintf(tw, "\t%d", d.views[t])
		}
		fmt.Fprintf(tw, "\t%d\t%d\t%d\t%s\t%s\t%d\n",
			d.clicks["app_store"], d.clicks["google_play"], d.clicks["open_app"],
			percent(d.notFound, opens), percent(d.failed, opens), d.bots)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	if len(bots) == 0 {
		return nil
	}
	names := make([]string, 0, len(bots))
	for k := range bots {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		if bots[names[i]] != bots[names[j]] {
			return bots[names[i]] > bots[names[j]]
		}
		return names[i] < names[j]
	})
	fmt.Fprintf(w, "\nbots, %d days:\n", days)
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, k := range names {
		fmt.Fprintf(tw, "  %s\t%d\n", k, bots[k])
	}
	return tw.Flush()
}

func percent(n, total int64) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f", float64(n)*100/float64(total))
}

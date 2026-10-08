package appointments

import (
	"testing"
	"time"
)

func TestSlots(t *testing.T) {
	loc, _ := time.LoadLocation("Africa/Douala")
	// Monday 12 October 2026
	day := time.Date(2026, 10, 12, 0, 0, 0, 0, loc)
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, loc)
	in := SlotInput{From: day, To: day, Loc: loc, Now: now, Duration: 45 * time.Minute, Buffer: 15 * time.Minute, Step: 30 * time.Minute,
		MinNotice: 12 * time.Hour, MaxAdvance: 60 * 24 * time.Hour,
		Rules: []Rule{{Weekday: 1, StartMinute: 9 * 60, EndMinute: 12 * 60}}}
	all := Slots(in)
	// 09:00 .. 11:00 starts (11:15 end <= 12:00); 11:30 would end 12:15
	if len(all) != 5 || all[0].Hour() != 9 || all[len(all)-1].Format("15:04") != "11:00" {
		t.Fatalf("got %v", all)
	}
	// A booking 10:00-10:45 (+15 buffer -> 11:00) blocks 09:30 (ends 10:30 incl. buffer overlap), 10:00 and 10:30.
	in.Busy = []busy{{start: day.Add(10 * time.Hour), end: day.Add(11 * time.Hour)}}
	got := Slots(in)
	want := []string{"09:00", "11:00"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i, w := range want {
		if got[i].Format("15:04") != w {
			t.Fatalf("got %v", got)
		}
	}
	// Minimum notice excludes slots too soon.
	in.Busy = nil
	in.Now = day.Add(1 * time.Hour)
	in.MinNotice = 9 * time.Hour // earliest 10:00
	if s := Slots(in); len(s) != 3 || s[0].Format("15:04") != "10:00" {
		t.Fatalf("notice: %v", s)
	}
	// Wrong weekday has no slots.
	in.Rules[0].Weekday = 2
	if s := Slots(in); len(s) != 0 {
		t.Fatalf("weekday: %v", s)
	}
}

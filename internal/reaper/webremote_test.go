package reaper

import "testing"

func TestParseTracks(t *testing.T) {
	input := "TRACK\t1\tKick\t8\t1.000000\t0.000000\t-1500\t-1500\t1.000000\t3\t0\t0\t0\t0\nTRACK\t2\tBass\t8\t0.500000\t-0.250000\t-1500\t-1500\t1.000000\t3\t1\t2\t1\t0"
	tracks := parseTracks(input)
	if len(tracks) != 2 {
		t.Fatalf("len(tracks) = %d, want 2", len(tracks))
	}
	if tracks[0].Name != "Kick" {
		t.Fatalf("tracks[0].Name = %q, want Kick", tracks[0].Name)
	}
	if !tracks[1].Mute || !tracks[1].Solo || !tracks[1].RecArm {
		t.Fatalf("expected mute/solo/rec-arm true for second track")
	}
}

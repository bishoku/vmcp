//go:build acceptance

package e2e

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

type acceptanceCapacityResult struct {
	PassingRPS int  `json:"passing_rps"`
	FailingRPS int  `json:"failing_rps"`
	Censored   bool `json:"upper_bound_not_found"`
}

func acceptanceCapacityRates(raw string) ([]int, error) {
	var out []int
	for _, item := range strings.Split(raw, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil || n < 100 || n > 20000 || (len(out) > 0 && n <= out[len(out)-1]) {
			return nil, fmt.Errorf("capacity rates must increase within 100..20000")
		}
		out = append(out, n)
	}
	if len(out) > 12 {
		return nil, fmt.Errorf("at most twelve capacity rates")
	}
	return out, nil
}

func acceptanceCapacitySearch(rates []int, seconds int, run func(string, int, int) acceptancePhase) acceptanceCapacityResult {
	result := acceptanceCapacityResult{}
	for _, rate := range rates {
		phase := run(fmt.Sprintf("capacity-%d", rate), rate, seconds)
		if !phase.SLOPass {
			result.FailingRPS = rate
			break
		}
		result.PassingRPS = rate
	}
	if result.FailingRPS == 0 {
		result.Censored = true
		return result
	}
	for i := 0; i < 3 && result.PassingRPS > 0 && result.FailingRPS-result.PassingRPS > 250; i++ {
		rate := (result.PassingRPS + result.FailingRPS) / 2 / 100 * 100
		run("recovery", 100, 30)
		phase := run(fmt.Sprintf("refine-%d", rate), rate, seconds)
		if phase.SLOPass {
			result.PassingRPS = rate
		} else {
			result.FailingRPS = rate
		}
	}
	return result
}

func TestCapacitySearchBoundsAndExhaustion(t *testing.T) {
	rates, err := acceptanceCapacityRates("500,1000,2000,4000")
	if err != nil {
		t.Fatal(err)
	}
	run := func(_ string, rps, _ int) acceptancePhase { return acceptancePhase{SLOPass: rps <= 1700} }
	result := acceptanceCapacitySearch(rates, 60, run)
	if result.PassingRPS != 1700 || result.FailingRPS != 2000 && result.FailingRPS != 1800 {
		t.Fatalf("unexpected bracket: %+v", result)
	}
	if result.Censored {
		t.Fatal("failed stage must bound capacity")
	}
	result = acceptanceCapacitySearch([]int{500, 1000}, 60, run)
	if !result.Censored || result.PassingRPS != 1000 {
		t.Fatalf("ceiling must be reported as unknown: %+v", result)
	}
	for _, raw := range []string{"", "1000,500", "500,500", "0", "20001", "x"} {
		if _, err := acceptanceCapacityRates(raw); err == nil {
			t.Fatalf("accepted invalid rates %q", raw)
		}
	}
}

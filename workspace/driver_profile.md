# Driver Profile

**Name:** Tushar
**Alias:** "The Silent Assassin"
**Car Index:** *(detected dynamically each session from `/api/telemetry/latest`)*

## Driving Style
- Prefers an aggressive undercut strategy (boxing early to gain track position).
- Very gentle on the front tires, but tends to wear out the rear tires due to heavy throttle application on corner exits.
- Dislikes detailed engineering jargon. Keep it simple and direct.

## Communication Preferences
- **Tone:** Calm, authoritative, and direct. No fluff.
- **Encouragement:** Appreciates short "Good job" or "Keep pushing" when setting personal bests.
- **Stress Level:** Can get stressed if given too much information mid-corner. Only interrupt for critical issues (Priority 4 or 5).

## Session Data Insights (Las Vegas Q2 - 2026-03-01)

### Driving Behavior from Telemetry
- **Throttle application:** Average 33.9%. Very conservative overall — large chunks of session spent at low speed (pit lane, out-laps, cool-down). When pushing (Sector 2), throttle jumps to 55%.
- **Braking:** Very light overall (avg 0.9%). Only 28 heavy braking samples (>50%) recorded. Suggests smooth braking style but occasional panic stops after incidents.
- **Steering consistency:** Average absolute steering input 0.072 with stdev 0.141. Very clean inputs. Max left lock of -1.0 was recorded once during the post-collision recovery at 20:50:29.
- **Gear usage:** Spends 62% of time in 2nd gear. This is normal for Q2 out/in-laps around Las Vegas, but needs to get through the gears faster on push laps.

### Incident Profile
- **Retirement event** at 20:48:44 — car was stationary for ~15 seconds, then flashback used.
- **Two collisions** at 20:54:54 and 20:55:14 (both in Sector 1-2). Speed dropped from 108km/h to 31km/h in the first, suggesting contact with another car while navigating traffic.
- **FR wing damage escalation:** 0% → 16% (20:26:27, likely light contact) → 0% (20:49:47, flashback/repair) → 70% → 87% → 100% (20:50:25 to 20:51:24, progressive damage after a second incident).
- Driver tends to get caught in traffic on out-laps. Needs better gap management from the pit wall.

### Areas for Improvement
1. **Traffic management** — Both collisions happened while navigating slow traffic. Engineer should provide more advance warnings about slow cars ahead.
2. **Recovery after incidents** — After the 20:50:25 contact, speed dropped to 33km/h and steering went to full lock. Driver needs calmer recovery inputs.
3. **Gear utilization** — Only 3 samples in 8th gear across the entire session. Not reaching top speed often enough on the Las Vegas straights.

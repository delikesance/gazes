# stream_saturation_pct: stream capacity

**Signal.** Not measured yet: there is no configured stream limit and no startup instrumentation. The rule shows `not_measured`; this page is the target description.

**When it exists.** Simultaneous playback sessions (`active_sessions` of `get_player_health`) over the capacity measured by a load test.

**What to do now.** Run a load test on the target machine, then record the limit; until then use the evening peak (20 h to 23 h, see the Visionnages page) as the planning figure.

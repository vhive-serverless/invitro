#!/usr/bin/env bash

# An archived acquisition marker is the authoritative boundary after which an
# E2-Synth cell and the surrounding suite may not advance automatically.
e2_synth_acquisition_started() {
    local archived_cell=$1
    [[ -f "$archived_cell/acquisition-started.marker" ]]
}

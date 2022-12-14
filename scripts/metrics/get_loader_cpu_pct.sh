#!/usr/bin/env bash

top -b -n 2 -d 0.2 -p $(pidof loader) | tail -1 | awk '{print $9" "$10}'
#!/bin/bash
# Copyright 2021 The Periph Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

set -eu

cd "$(dirname $0)"

# Set it to -v for verbosity.
QUIET=-q

if [ -f venv/bin/esphome ]; then
  exit 0
fi

if [ ! -d venv ]; then
  mkdir venv
fi

if [ ! -f ./venv/bin/activate ]; then
  python3 -m venv venv
fi

echo "- Activating virtualenv"
source venv/bin/activate

echo "- Installing requirements"
pip3 install $QUIET -U -r requirements.txt

echo ""
echo "Congratulations! Everything is inside ./venv/"
echo "To access esphome, run:"
echo "  source venv/bin/activate"

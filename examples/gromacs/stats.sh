#!/bin/bash

set -e

GROMACS_RESULTS_PATH=${GROMACS_RESULTS_PATH:-results}
GROMACS_ANALYSIS_FLAGS=${GROMACS_ANALYSIS_FLAGS:--p -l}

data_root=$(ls -d $GROMACS_RESULTS_PATH)

csv=$(mktemp)
./aggregate.sh $data_root | sed '/,,/d ' > $csv
./analysis.py ${GROMACS_ANALYSIS_FLAGS} $csv
rm $csv


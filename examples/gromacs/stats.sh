#!/bin/bash

set -e

source conf.env

GROMACS_ANALYSIS_FLAGS=${GROMACS_ANALYSIS_FLAGS:--p -l}

data_root=$(ls -d ${KFGROMACS_RESULTS_PATH})

csv=${KFGROMACS_RESULTS_PATH}/grand.csv
./aggregate.sh $data_root | sed '/,,/d ' > $csv
./analysis.py ${GROMACS_ANALYSIS_FLAGS} $csv

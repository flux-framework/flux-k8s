#!/bin/bash

set -e

exp_root=$(ls -d $1)

n_nodes=$(cat $exp_root/cluster-nodes | egrep -v "master|NotReady|SchedulingDisabled" | tail -n +1 | wc -l)

hpath=$(ls -d1 $exp_root/*mpi | head -n 1)
if find $hpath -name "*-run1" > /dev/null
then
	hfile=$(ls -1 $hpath/*-run1 | head -n 1)
	echo n-nodes,mode,ranks,$(./summary.sh $hfile | tail -n 2 | head -n 1)
else
	echo "> aborting as no base file ($hpath/*-run1) was not found"
	exit 1
fi

for d in $(ls -d $exp_root/*mpi)
do
	ranks=$(basename $d | grep -o "[0-9]*")
	for m in default flux
	do
		if [[ -f $d/$m-run1 ]]
		then
			for s in $(ls $d/$m-run*)
			do
				echo $n_nodes,$m,$ranks,$(./summary.sh $s | tail -n 1)
			done
		fi
	done
done

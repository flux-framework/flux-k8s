#!/bin/bash

SCHED=$1
OUTFILE=$2
OUTPATH=$3
FLUXYAML=$4
COMMENT="#"

if [[ -z $SCHED || -z $OUTFILE || -z $OUTPATH ]]; then
	echo "usage: ./launch.sh <flux|default> <output-file> <output-path> <kubeflux-deployment-file>"
	exit 1
fi

if [[ $SCHED == "flux" && -z $FLUXYAML ]]; then
	echo "path to KubeFlux yaml missing!"
	echo "usage: ./launch.sh <flux|default> <output-file> <output-path> <kubeflux-deployment-file>"
fi

if [[ $SCHED == "flux" ]]; then
	echo Redeploy kubeflux because.
	kubectl delete -f ${FLUXYAML}
	sleep 2s
	kubectl create -f ${FLUXYAML}
	COMMENT=''
fi

echo Saving cluster worker nodes information
kubectl get nodes -o wide > ${OUTPATH}/cluster-nodes

for NP in 1 2 4 8 16 32 48 64; do 
	STEPS=$(( 100 * ${NP} ))
	if [[ $NP -gt 32 ]]; then
		STEPS=$(( 100 * 32 ))
	fi
	echo Steps: $STEPS

	OUTDIR=${OUTPATH}/${NP}mpi
	mkdir -p ${OUTDIR}

	sed -e s/%STEPS%/${STEPS}/g -e s/%NP%/${NP}/g -e "s/\(schedulerName: [[:alpha:]]*\)/${COMMENT}\1/g" gromacs-mpijob-benchmark.yaml > ${OUTDIR}/deployment.yaml

	echo Start with $NP MPI ranks:

	for i in {1..5}; do
		echo Creating deployment with $NP MPI Ranks
		kubectl create -f ${OUTDIR}/deployment.yaml
		sleep 10s

		LAUNCHER=`kubectl get po -o name | grep launcher`
		
		while [ `kubectl get statefulsets | grep worker | awk -F' ' '{print $2}' | awk -F'/' '{print $1}'` -lt $NP ]; do
			echo Waiting to dump pods allocation
			sleep 10s
		done

		echo Dumping pods allocation
		kubectl get po -o wide | grep gromacs >> ${OUTDIR}/${OUTFILE}-run${i}


		while [ `kubectl get pods | grep launcher | awk -F' ' '{print $3}'` != "Completed" ]; do
			status=$(kubectl get pods | grep launcher | awk -F' ' '{print $3}')
			echo Waiting for pod ${LAUNCHER}, still $status
			sleep 30s
		done
		echo Run completed 

		sleep 10s
		
		kubectl logs ${LAUNCHER} >>  ${OUTDIR}/${OUTFILE}-run${i}
		kubectl delete -f ${OUTDIR}/deployment.yaml

		echo Need to wait for pods to go away
		while [ `kubectl get pods | grep gromacs-benchmark-worker  | wc -l` -gt 0 ]; do
			echo Still `kubectl get pods | grep gromacs-benchmark-worker  | wc -l` pods running
			sleep 10s
		done
		if [[ $SCHED == "flux" ]]; then
			echo Dumping kubeflux logs
			fluxpod=`kubectl get po -n kube-system -o name | grep flux`
			kubectl logs $fluxpod -n kube-system >> ${OUTDIR}/kubeflux-log
			echo Need to restart kubeflux
			kubectl delete -f ${FLUXYAML}
			sleep 5s
			kubectl create -f ${FLUXYAML}
		fi
		echo Done. 
	done
	echo Finished with $NP ranks
done
kubectl delete -f ${FLUXYAML}
echo Finished. Bye.

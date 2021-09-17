#!/bin/bash

source conf.env

SCHED=$1
OUTFILE=$2
FLUXYAML=$3
COMMENT="#"

if [[ -z $SCHED || -z $OUTFILE ]]; then
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
mkdir -p $KFGROMACS_RESULTS_PATH
kubectl get nodes -o wide > ${KFGROMACS_RESULTS_PATH}/cluster-nodes
nnodes=$(tail -n +2 ${KFGROMACS_RESULTS_PATH}/cluster-nodes | wc -l)

for NP in $(echo ${KFGROMACS_MPI_RANKS} | sed 's/,/ /g'); do
	if [[ $NP -gt ${nnodes} ]]; then
		STEPS=$(( 100 * ${nnodes} ))
	else
		STEPS=$(( 100 * ${NP} ))
	fi
	echo Steps: $STEPS

	OUTDIR=${KFGROMACS_RESULTS_PATH}/${NP}mpi
	mkdir -p ${OUTDIR}

	sed -e s/%STEPS%/${STEPS}/g -e s/%NP%/${NP}/g -e "s/\(schedulerName: [[:alpha:]]*\)/${COMMENT}\1/g" gromacs-mpijob-benchmark.yaml > ${OUTDIR}/deployment.yaml

	echo Start with $NP MPI ranks:

	for i in $(seq 1 ${KFGROMACS_REPETITIONS}); do
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

#!/bin/bash

node_idx=7

echo "> reading input file: $1"
infile=$(ls $1)

grep_pods="cat $infile | grep gromacs | grep 'worker-' | egrep 'Running|ContainerCreating|Terminating'"

perfs=$(grep -A 3 "Core t (s)" $infile | tail -n 1 | awk '{printf("%s,%s",$2,$3)}' )
wtime=$(grep -A 1 "Core t (s)" $infile | tail -n 1 | awk '{printf("%s,%s,%s",$2,$3,$4)}' )
imbalance=$(grep "Average load imbalance" $infile | grep -o "[0-9]*\.[0-9]*%" | sed 's/%//')

echo "> dumping number of scheduled pods per node:"
declare -A pps
max_node_occupation=0
echo -e "npods node subnet"
cmd="$grep_pods | awk '{print \$$node_idx}' | sort | uniq -c | sort -g"
while read -r line
do
    node_occupation=$(echo $line | awk '{print $1}')
    if [[ $node_occupation -gt $max_node_occupation ]]
    then
        max_node_occupation=$node_occupation
    fi
    subnet=$(echo $line | awk '{print $2}' | cut -f 3 -d '.')
    if [ ! ${pps[$subnet]+_} ]
    then
        pps[$subnet]=0
    fi
    pps[$subnet]=$(( ${pps[$subnet]} + ${node_occupation} ))
    echo -e "$line $subnet"
done <<<$(eval $cmd)

echo "> subnet summary"
n_subnets=${#pps[@]}
max_subnet_size=0
for subnet in "${!pps[@]}"
do
    subnet_size=${pps[$subnet]}
    if [[ $subnet_size -gt $max_subnet_size ]]
    then
        max_subnet_size=${subnet_size}
    fi
    echo $subnet: "$subnet_size"
done


echo "> computing the summary"
cmd="$grep_pods | awk '{print \$$node_idx}' | wc -l"
n_pods=$(eval $cmd)
cmd="$grep_pods | awk '{print \$$node_idx}' | sort | uniq | wc -l"
n_busy_nodes=$(eval $cmd)

echo "n-pods,n-busy-nodes,n-subnets,max-subnet-size,max-node-occ,ns/day,hour/ns,core-sec,wall-sec,%,imbalance"
echo "$n_pods,$n_busy_nodes,$n_subnets,$max_subnet_size,$max_node_occupation,$perfs,$wtime,$imbalance"

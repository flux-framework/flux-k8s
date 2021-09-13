FROM gromacs/gromacs

WORKDIR /data
RUN wget http://public-ehs.oss-cn-hangzhou.aliyuncs.com/packages/water_GMX50_bare.tar.gz
RUN tar -xvf water_GMX50_bare.tar.gz

# create a gromacs topology file
WORKDIR /data/water-cut1.0_GMX50_bare/1536
RUN gmx grompp -f pme.mdp -c conf.gro -p topol.top -o topol_pme.tpr

# test topology file
RUN /usr/local/openmpi/bin/mpirun -np 1 --allow-run-as-root gmx mdrun -s topol_pme.tpr -ntomp 1 -resethway -noconfout -nsteps 10 -v -pin on


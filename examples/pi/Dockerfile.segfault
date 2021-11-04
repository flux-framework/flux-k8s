FROM ubuntu:latest
MAINTAINER Eduardo Arango <carlos.arango.gutierrez@correounivalle.edu.co>

RUN apt-get update
RUN apt-get install -y bash wget build-essential gcc g++ git
RUN git clone https://github.com/ArangoGutierrez/Pi.git
WORKDIR Pi
RUN g++ -fopenmp pi.c -o pi
RUN cp pi /bin

ENTRYPOINT ["pi"]

FROM ubuntu:latest

WORKDIR /app

RUN apt-get update && \
    apt-get install git -y && \ 
    apt-get update && \
    apt-get install -y curl && \
    curl -O https://dl.google.com/go/go1.20.1.linux-amd64.tar.gz && \
    tar -xvf go1.20.1.linux-amd64.tar.gz && \
    mv go /usr/local 

ENV PATH="/usr/local/go/bin:${PATH}"

RUN git clone https://github.com/ctSkennerton/mk.git && \
    cd mk && \
    go build && \
    mv mk /bin/

RUN git clone git://git.z3bra.org/partage.git

RUN cd partage && \
    go get && \
    go install

RUN sed -i "s/}/}\`/g" partage/config.mk && \
    sed -i "s/127.0.0.1:9000/0.0.0.0:9000/g" partage/example/partage.conf

RUN mkdir -p partage/example/files && \
    mkdir -p partage/example/meta && \
    chmod 777 partage/example

RUN cd partage && \
    mk && \
    mk install

CMD cd partage && partage -v -f example/partage.conf

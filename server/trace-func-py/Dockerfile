FROM ubuntu:focal

RUN apt update -y
RUN apt install -y python3 python3-pip make git python-is-python3
RUN pip install grpcio grpcio-tools wheel 

WORKDIR /app
COPY . ./
WORKDIR /app/MockS3
RUN make install

WORKDIR /app
EXPOSE 80
EXPOSE 60001

ARG FUNC_TYPE
ENV FUNC_TYPE_ENV=${FUNC_TYPE}
ARG FUNC_PORT
ENV FUNC_PORT_ENV=${FUNC_PORT}

CMD python3 trace_func.py --port ${FUNC_PORT_ENV} --type ${FUNC_TYPE_ENV}
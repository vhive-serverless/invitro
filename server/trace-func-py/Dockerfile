FROM debian:stable-slim

RUN apt update -y
RUN apt install -y python3 python3-pip
RUN pip install grpcio grpcio-tools --break-system-packages

WORKDIR /app
COPY * ./

EXPOSE 80

ARG FUNC_TYPE
ENV FUNC_TYPE_ENV=${FUNC_TYPE}
ARG FUNC_PORT
ENV FUNC_PORT_ENV=${FUNC_PORT}

CMD python3 trace_func.py --port ${FUNC_PORT_ENV} --type ${FUNC_TYPE_ENV}
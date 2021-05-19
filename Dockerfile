FROM alpine:3.8

EXPOSE 7659 7651 7653 7650

RUN mkdir /data
COPY ./build/oklog /oklog
RUN ["chmod", "775", "/oklog"]

ENTRYPOINT ["/oklog"]
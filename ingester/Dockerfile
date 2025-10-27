FROM docker.redpanda.com/redpandadata/connect

# add our connect.yaml to the stock image
COPY ./connect.yaml ./start.sh ./

# clear enrypoint because we use launch script
ENTRYPOINT []

# run the launch script
CMD ["./start.sh"]

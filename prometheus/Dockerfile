FROM prom/prometheus:latest

USER root

COPY prometheus/start.sh /start.sh
RUN chmod +x /start.sh

EXPOSE 9090

ENTRYPOINT ["/start.sh"]

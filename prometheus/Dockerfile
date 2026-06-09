FROM prom/prometheus@sha256:69f5241418838263316593f7274a304b095c40bcf22e57272865da91bd60a8ac

USER root

COPY --chown=nobody:nobody start.sh /start.sh
RUN chmod +x /start.sh
USER nobody

EXPOSE 9090

ENTRYPOINT ["/start.sh"]

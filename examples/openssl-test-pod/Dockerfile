FROM docker.io/fedora:40

RUN dnf install -y nginx openssl \
    && dnf clean all \
    && rm -rf /var/cache/dnf

COPY nginx.conf /etc/nginx/nginx.conf
COPY entrypoint.sh /entrypoint.sh
COPY html/healthz /var/www/html/healthz
COPY html/index.html /var/www/html/index.html
COPY html/message /var/www/html/message
COPY html/api/items /var/www/html/api/items
COPY html/api/echo /var/www/html/api/echo

# OpenShift/Kubernetes may run an arbitrary UID without group 0 — use world-readable
# paths for this self-signed test cert and nginx temp dirs.
RUN mkdir -p /tmp/nginx/{client_body,proxy,fastcgi,uwsgi,scgi} /tmp/tls /var/www/html/api \
    && openssl req -x509 -newkey rsa:2048 -nodes \
         -keyout /tmp/tls/key.pem -out /tmp/tls/cert.pem \
         -days 365 -subj "/CN=openssl-test-pod" \
         -addext "subjectAltName=DNS:openssl-test,DNS:localhost" \
    && chmod +x /entrypoint.sh \
    && chmod -R a+rwX /tmp/nginx /tmp/tls /var/www/html /var/log/nginx /var/lib/nginx \
    && chmod a+r /tmp/tls/cert.pem /tmp/tls/key.pem

USER 65534:0
EXPOSE 8443
ENTRYPOINT ["/entrypoint.sh"]

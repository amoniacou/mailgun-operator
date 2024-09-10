FROM gcr.io/distroless/static:nonroot
ENV SUMMARY="Mailgun Operator Container Image." \
    DESCRIPTION="This Docker image contains Mailgun Operator."

LABEL summary="$SUMMARY" \
      description="$DESCRIPTION" \
      io.k8s.display-name="$SUMMARY" \
      io.k8s.description="$DESCRIPTION" \
      name="Mailgun Operator" \
      vendor="Amoniac OU" \
      url="https://amoniac.eu/" \
      version="$VERSION" \
      release="1"

WORKDIR /
USER nonroot:nonroot
COPY --chown=nonroot:nonroot --chmod=0755 dist/manager_linux_${TARGETARCH}*/manager .
ENTRYPOINT ["/manager"]

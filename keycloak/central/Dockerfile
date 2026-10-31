FROM quay.io/keycloak/keycloak:26.7.0

# Chooses the DC from the email domain. Keep this release matched to the Keycloak version.
ADD --checksum=sha256:c7f762ff5739443bf9d0862821b6b86328ef3cc02f5131ba93a65662215e4151 --chmod=644 \
    https://github.com/sventorben/keycloak-home-idp-discovery/releases/download/v26.2.2/keycloak-home-idp-discovery.jar \
    /opt/keycloak/providers/keycloak-home-idp-discovery.jar

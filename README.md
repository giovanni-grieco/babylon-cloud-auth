# Babylon Auth

Un laboratorio per testare un architettura di autenticazione centralizzata per sistemi distribuiti

## Keycloak
Per configurare i segreti e utilizzarli nel realm.json file di import per keycloak, si aspetta un segreto in docker compose come <nome_realm>_<nome_segreto> per farlo finire nel vault con ${vault.<nome_segreto>}
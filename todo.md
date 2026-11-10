- Implementare strato di sicurezza per control plane: control-plane-keycloak, oauth-proxy, traefik gateway che mette in sicurezza le rotte per il control plane. Rendere gli altri keycloak, clients del control plane keycloak per ottenere i token di accesso da presentare a traefik.

- Implementare un sistema di notifiche utente che gli dice quando la riconciliazione è state effettuata con successo se ha deciso di cambiare username. Il control plane fa la verifica del caso quando la riconciliazione avviene

- Implementare il control plane per l'amministratore (pagine web etc)
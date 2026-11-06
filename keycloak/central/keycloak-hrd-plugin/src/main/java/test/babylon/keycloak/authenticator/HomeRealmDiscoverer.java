package test.babylon.keycloak.authenticator;


import com.fasterxml.jackson.databind.JsonNode;
import jakarta.ws.rs.core.MultivaluedMap;
import jakarta.ws.rs.core.Response;
import org.apache.http.client.config.RequestConfig;
import org.keycloak.authentication.AuthenticationFlowContext;
import org.keycloak.authentication.AuthenticationFlowError;
import org.keycloak.authentication.AuthenticationProcessor;
import org.keycloak.authentication.Authenticator;
import org.keycloak.authentication.authenticators.browser.AbstractUsernameFormAuthenticator;
import org.keycloak.http.simple.SimpleHttp;
import org.keycloak.http.simple.SimpleHttpResponse;
import org.keycloak.models.*;
import org.keycloak.protocol.oidc.OIDCLoginProtocol;

import org.jboss.logging.Logger;
import org.keycloak.services.Urls;
import org.keycloak.services.managers.ClientSessionCode;
import org.keycloak.sessions.AuthenticationSessionModel;

import java.net.URI;
import java.util.Map;

public class HomeRealmDiscoverer implements Authenticator{

    private static final Logger LOG = Logger.getLogger(HomeRealmDiscoverer.class);

    static final String CFG_ORACLE_URL = "oracle.url";
    static final String CFG_TIMEOUT_MS = "oracle.timeoutMs";

    @java.lang.Override
    public void authenticate(AuthenticationFlowContext context) {
        String hint = context.getAuthenticationSession().getClientNote(OIDCLoginProtocol.LOGIN_HINT_PARAM);
        if (hint != null && !hint.isBlank()) {
            discover(context, hint.trim());   // same logic action() uses: oracle → redirect or central login
            return;
        }
        context.challenge(context.form().createLoginUsername());
    }

    @java.lang.Override
    public void action(AuthenticationFlowContext context) {
        MultivaluedMap<String, String> form = context.getHttpRequest().getDecodedFormParameters();
        String username = form.getFirst("username");

        if (username == null || username.isBlank()) {
            context.failureChallenge(AuthenticationFlowError.INVALID_USER,
                    context.form().setError("missingUsernameMessage").createLoginUsername());
            return;
        }
        discover(context, username.trim());
    }

    @java.lang.Override
    public boolean requiresUser() {
        return false;
    }

    @java.lang.Override
    public boolean configuredFor(KeycloakSession session, RealmModel realm, UserModel user) {
        return true;
    }

    @java.lang.Override
    public void setRequiredActions(KeycloakSession session, RealmModel realm, UserModel user) {

    }

    @java.lang.Override
    public void close() {

    }

    private void discover(AuthenticationFlowContext context, String hint){
        context.getAuthenticationSession()
                .setAuthNote(AbstractUsernameFormAuthenticator.ATTEMPTED_USERNAME, hint);


        String idpAlias;
        try{
            idpAlias = askOracle(context, hint);
        } catch (Exception e){
            // Oracle down/misconfigured: default to logging in on central
            LOG.warnf(e, "HRD oracle call on %s failed for realm %s, falling back to central login", CFG_ORACLE_URL, context.getRealm().getName());
            fallBackToCentral(context, hint);
            return;
        }
        if (idpAlias!=null && !idpAlias.equals("central")){
            LOG.infof("Home realm %s found for %s", idpAlias, hint);
            redirectToIdp(context, idpAlias, hint);
        }else{
            fallBackToCentral(context, hint);
        }

    }

    /**
     * Not a B2B user (or no answer): step aside. discover-dc does not succeed,
     * so the flow moves on to the next ALTERNATIVE (b2c-login on central).
     */
    private void fallBackToCentral(AuthenticationFlowContext context, String hint){
        // UsernamePasswordForm pre-fills the username only from the login_hint client note
        context.getAuthenticationSession().setClientNote(OIDCLoginProtocol.LOGIN_HINT_PARAM, hint);
        context.attempted();
    }

    private String askOracle(AuthenticationFlowContext context, String username) throws Exception {
        AuthenticatorConfigModel cfg = context.getAuthenticatorConfig();
        if (cfg == null || cfg.getConfig().get(CFG_ORACLE_URL) == null) {
            throw new IllegalStateException("HRD authenticator not configured: missing " + CFG_ORACLE_URL);
        }
        String url = cfg.getConfig().get(CFG_ORACLE_URL);
        if (url.isBlank()) {
            throw new IllegalStateException("HRD authenticator not configured: " + CFG_ORACLE_URL + " is empty");
        }
        int timeout = Integer.parseInt(cfg.getConfig().getOrDefault(CFG_TIMEOUT_MS, "2000"));

        // Uses Keycloak's shared, pooled HTTP client (truststore/proxy settings apply)
        RequestConfig rc = RequestConfig.custom()
                .setConnectTimeout(timeout)
                .setConnectionRequestTimeout(timeout)
                .setSocketTimeout(timeout)
                .build();

        try (SimpleHttpResponse resp = SimpleHttp.create(context.getSession())
                .withRequestConfig(rc)
                .doPost(url)
                .acceptJson()
                .json(Map.of("login_hint", username))
                .asResponse()) {

            if (resp.getStatus() != 200) {
                throw new IllegalStateException("Oracle returned HTTP " + resp.getStatus());
            }
            JsonNode idp = resp.asJson().get("idp");
            return (idp == null || idp.isNull() || idp.asText().isBlank()) ? null : idp.asText();
        }
    }

    private void redirectToIdp(AuthenticationFlowContext context, String alias, String hint){
        IdentityProviderModel idp = context.getSession().identityProviders().getByAlias(alias);
        if (idp == null || !idp.isEnabled()) {
            LOG.warnf("Oracle returned unknown/disabled IdP '%s', falling back to central login", alias);
            fallBackToCentral(context, hint);
            return;
        }

        AuthenticationSessionModel authSession = context.getAuthenticationSession();
        // The code ties the broker callback back to *this* auth session
        String accessCode = new ClientSessionCode<>(context.getSession(), context.getRealm(), authSession)
                .getOrGenerateCode();
        String clientData = AuthenticationProcessor.getClientData(context.getSession(), authSession);

        URI location = Urls.identityProviderAuthnRequest(
                context.getUriInfo().getBaseUri(),
                alias,
                context.getRealm().getName(),
                accessCode,
                authSession.getClient().getClientId(),
                authSession.getTabId(),
                clientData,
                hint);                 // pre-fills the username on the remote login page

        // forceChallenge: send this response no matter what the rest of the flow says
        context.forceChallenge(Response.seeOther(location).build());
    }

}

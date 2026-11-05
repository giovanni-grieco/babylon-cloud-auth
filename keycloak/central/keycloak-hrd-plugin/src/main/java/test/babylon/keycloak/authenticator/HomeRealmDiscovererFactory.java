package test.babylon.keycloak.authenticator;

import org.keycloak.Config;
import org.keycloak.authentication.Authenticator;
import org.keycloak.authentication.AuthenticatorFactory;
import org.keycloak.models.AuthenticationExecutionModel;
import org.keycloak.models.KeycloakSession;
import org.keycloak.models.KeycloakSessionFactory;
import org.keycloak.provider.ProviderConfigProperty;
import org.keycloak.provider.ProviderConfigurationBuilder;

import java.util.List;

public class HomeRealmDiscovererFactory implements AuthenticatorFactory {

    public static final String PROVIDER_ID = "babylon-hrd";

    // The authenticator holds no state, so one instance is enough
    private static final HomeRealmDiscoverer SINGLETON = new HomeRealmDiscoverer();

    private static final AuthenticationExecutionModel.Requirement[] REQUIREMENTS = {
            AuthenticationExecutionModel.Requirement.REQUIRED,
            AuthenticationExecutionModel.Requirement.ALTERNATIVE,
            AuthenticationExecutionModel.Requirement.DISABLED
    };

    @Override
    public String getId() {
        return PROVIDER_ID;
    }

    @Override
    public Authenticator create(KeycloakSession session) {
        return SINGLETON;
    }

    @Override
    public String getDisplayType() {
        return "Babylon Home Realm Discovery";     // name shown in "Add step"
    }

    @Override
    public String getHelpText() {
        return "Asks for the username, queries a remote oracle and redirects to the user's home IdP.";
    }

    @Override
    public String getReferenceCategory() {
        return "hrd";
    }

    @Override
    public boolean isConfigurable() {
        return true;                               // enables the ⚙ config in the flow editor
    }

    @Override
    public boolean isUserSetupAllowed() {
        return false;
    }

    @Override
    public AuthenticationExecutionModel.Requirement[] getRequirementChoices() {
        return REQUIREMENTS;
    }

    @Override
    public List<ProviderConfigProperty> getConfigProperties() {
        return ProviderConfigurationBuilder.create()
                .property()
                .name(HomeRealmDiscoverer.CFG_ORACLE_URL)
                .label("Oracle URL")
                .helpText("POST endpoint returning {\"idp\": \"<alias>\"} or {\"idp\": null}")
                .type(ProviderConfigProperty.STRING_TYPE)
                .add()
                .property()
                .name(HomeRealmDiscoverer.CFG_TIMEOUT_MS)
                .label("Oracle timeout (ms)")
                .helpText("Connect/read timeout for the oracle call")
                .type(ProviderConfigProperty.STRING_TYPE)
                .defaultValue("2000")
                .add()
                .build();
    }

    @Override
    public void init(Config.Scope config) {
        // server-level config (kc.sh --spi-authenticator-babylon-hrd-...) would be read here
    }

    @Override
    public void postInit(KeycloakSessionFactory factory) {
    }

    @Override
    public void close() {
    }
}

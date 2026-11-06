package contributor_test

import (
	"testing"

	"github.com/julez-dev/chatuino/contributor"
	"github.com/stretchr/testify/require"
)

func TestIsContributorRecognizesEveryEmbeddedTwitchLogin(t *testing.T) {
	t.Parallel()

	all := contributor.GetAll()
	require.NotEmpty(t, all)

	for _, c := range all {
		require.NotEmpty(t, c.Twitch, "contributor %q must carry a twitch login", c.GitHubUser)
	}

	for _, login := range []string{"julezdev", "axm2", "pera_el", "JulezDev"} {
		require.True(t, contributor.IsContributor(login), "%q must be recognized as a contributor", login)
	}

	require.False(t, contributor.IsContributor("not-a-contributor"))
}

// Create a certain action if a candidate does not exist using transaction
// that is valid?

newCandidate := breezy.Event(breezy.CandidateInStage("systems-engineer", "Coding Challenge"))

// process variables?
Process(Spec{
	Watch: newCandidate,
	Run: CreateKey("slack-channel", newCandidate.Email).Success(
		Sequence(
			// Imperative vs converging or both?
			slack.ChannelExists(uuid.New()),
			slack.RestrictedUserInvited(self.Prev().GetValue("name")),
		),
	),
})

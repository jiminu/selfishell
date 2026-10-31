package report

// Affirmative reports whether a prompt answer is an explicit yes.
func Affirmative(s string) bool { return s == "y" || s == "Y" || s == "yes" || s == "YES" }

// Negative reports whether a prompt answer is an explicit no.
func Negative(s string) bool { return s == "n" || s == "N" || s == "no" || s == "NO" }

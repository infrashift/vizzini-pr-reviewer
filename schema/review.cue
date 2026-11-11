// Canonical contract for every LLM review response (ADR-0003). Definitions are
// closed: unknown fields are rejected, not ignored.
package review

#Severity: "Critical" | "Major" | "Minor" | "Trivial"
#Category: "Security" | "Performance" | "Logic" | "Maintainability"

#Finding: {
	file:        string & !=""
	start_line:  int & >=1
	end_line:    int & >=start_line
	severity:    #Severity
	category:    #Category
	title:       string & !=""
	body:        string & !=""
	suggestion?: string
}

#ReviewOutput: {
	summary:    string
	risk_score: int & >=0 & <=100
	findings: [...#Finding]
}

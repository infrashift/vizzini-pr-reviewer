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

// Describe mode: a pull request description.
#PRType: "Feature" | "Bugfix" | "Refactor" | "Docs" | "Tests" | "Chore"

#FileChange: {
	file:        string & !=""
	description: string & !=""
}

#PRDescription: {
	title:   string & !=""
	type:    #PRType
	summary: string & !=""
	changes: [...#FileChange]
}

// Improve mode: concrete code suggestions.
#CodeSuggestion: {
	file:          string & !=""
	start_line:    int & >=1
	end_line:      int & >=start_line
	category:      #Category
	summary:       string & !=""
	existing_code: string
	improved_code: string & !=""
}

#ImproveOutput: {
	suggestions: [...#CodeSuggestion]
}

package repoctx

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestDefinedNames(t *testing.T) {
	for _, tc := range []struct {
		path, line string
		want       []string
	}{
		{"a.go", "func (s *Store) FindUser(ctx context.Context, id int) (*User, error) {", []string{"FindUser"}},
		{"a.go", "func parseToken(s string) string {", []string{"parseToken"}},
		{"a.go", "type Store struct {", []string{"Store"}},
		{"a.go", "if x := find(y); x != nil {", nil},
		{"src/OrderRepository.cs", "    public async Task<IReadOnlyList<Order>> FindByCustomerAsync(string customer, CancellationToken ct)", []string{"FindByCustomerAsync"}},
		{"src/OrderRepository.cs", "public sealed class OrderRepository(DbConnection connection)", []string{"OrderRepository"}},
		{"src/A.cs", "        return orders.FindByCustomerAsync(customer, ct);", nil},
		{"src/A.cs", "        if (Validate(order)) {", nil},
		{"src/A.cs", "        var x = new Order(1, \"a\", 2m, DateTime.Now);", nil},
		{"Main.java", "    private static String normalize(String input) throws IOException {", []string{"normalize"}},
		{"Main.java", "public interface Repository<T> {", []string{"Repository"}},
		{"app.ts", "export async function saveAudit(event: AuditEvent): Promise<void> {", []string{"saveAudit"}},
		{"app.ts", "export const mergeSettings = (target: Settings, source: Settings) => {", []string{"mergeSettings"}},
		{"app.ts", "export class AuditLog {", []string{"AuditLog"}},
		{"app.ts", "  async flush(limit: number): Promise<void> {", []string{"flush"}},
		{"app.ts", "  saveAudit({ actor, action });", nil},
		{"app.py", "    async def load_profile(self, user_id):", []string{"load_profile"}},
		{"app.py", "class Billing(Base):", []string{"Billing"}},
		{"app.rb", "  def self.charge!(amount)", []string{"charge!"}},
		{"lib.rs", "pub fn parse_header(input: &str) -> Result<Header, Error> {", []string{"parse_header"}},
		{"README.md", "def not_code():", nil},
	} {
		if got := definedNames(familyOf(tc.path), tc.line); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %q defines %v, want %v", tc.path, tc.line, got, tc.want)
		}
	}
}

func TestUsedNames(t *testing.T) {
	for _, tc := range []struct {
		path, line string
		want       []string
	}{
		{"src/A.cs", "        var found = await orders.FindByCustomerAsync(customer, ct);", []string{"FindByCustomerAsync"}},
		{"src/A.cs", "        var o = new OrderRepository(conn); if (Validate(o)) Log(o);", []string{"OrderRepository", "Validate"}},
		{"src/A.cs", `        cmd.CommandText = "SELECT Lookup(x)";`, nil},
		{"app.ts", "  saveAudit({ actor, action: `promote ${id}` }); // retryLater(x)", []string{"saveAudit"}},
		{"app.ts", "  for (const u of users) if (isAdmin(u)) grant(u);", []string{"isAdmin", "grant"}},
		{"app.py", "    profile = fetch_profile(user_id) or default_profile()", []string{"fetch_profile", "default_profile"}},
		{"a.go", `	u, err := s.FindUser(ctx, id); fmt.Println(len(u))`, []string{"FindUser"}},
		{"a.go", "func parseToken(s string) string {", nil},
	} {
		if got := usedNames(familyOf(tc.path), tc.line); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %q uses %v, want %v", tc.path, tc.line, got, tc.want)
		}
	}
}

func repo() fstest.MapFS {
	return fstest.MapFS{
		"src/Repositories/OrderRepository.cs": {Data: []byte(`using System.Data.Common;

namespace Shop.Repositories;

public sealed class OrderRepository(DbConnection connection)
{
    public async Task<IReadOnlyList<Order>> FindByCustomerAsync(string customer, CancellationToken ct)
    {
        await using var cmd = connection.CreateCommand();
        cmd.CommandText = "SELECT id FROM orders WHERE customer = @customer";
        return await Read(cmd, ct);
    }

    private static Task<IReadOnlyList<Order>> Read(DbCommand cmd, CancellationToken ct) => throw new NotImplementedException();
}
`)},
		"src/Controllers/ReportsController.cs": {Data: []byte(`namespace Shop.Controllers;

public sealed class ReportsController(OrderRepository orders)
{
    public Task<IReadOnlyList<Order>> Export(string customer, CancellationToken ct) =>
        orders.FindByCustomerAsync(customer, ct);
}
`)},
		"web/audit.ts": {Data: []byte(`export async function saveAudit(event: AuditEvent): Promise<void> {
  await db.insert(event);
}
`)},
		"web/users.ts": {Data: []byte(`import { saveAudit } from "./audit.js";

export function promote(id: string): void {
  saveAudit({ actor: id });
}
`)},
		"vendor/lib/OrderRepository.cs":   {Data: []byte("public class OrderRepository { public Task FindByCustomerAsync() {} }\n")},
		"node_modules/x/index.ts":         {Data: []byte("export function saveAudit() {}\n")},
		".git/config":                     {Data: []byte("[core]\n")},
		"docs/notes.md":                   {Data: []byte("FindByCustomerAsync(customer) is documented here\n")},
		"src/Big.cs":                      {Data: []byte(strings.Repeat("// filler\n", 300) + "public Task FindByCustomerAsync(int x) { }\n")},
		"src/Repositories/OrderRepo.g.cs": {Data: []byte("public Task FindByCustomerAsync(long x) { }\n")},
	}
}

func TestScan(t *testing.T) {
	want := Want{
		Used:    map[Family]map[string]bool{CFamily: {"FindByCustomerAsync": true}, JS: {"saveAudit": true}},
		Defined: map[Family]map[string]bool{CFamily: {"FindByCustomerAsync": true}},
	}
	got, err := Scan(repo(), want, Limits{MaxFileBytes: 2000, Exclude: []string{"*.g.cs"}})
	if err != nil {
		t.Fatal(err)
	}
	lines := make([]string, 0, len(got))
	for _, s := range got {
		lines = append(lines, s.Kind+" "+s.Symbol+" "+s.Path+":"+itoa(s.Start)+"-"+itoa(s.End))
	}
	want2 := []string{
		// The definition, from its declaration to the end of its body.
		"definition FindByCustomerAsync src/Repositories/OrderRepository.cs:7-12",
		// Big.cs is over MaxFileBytes; vendor/, node_modules/, .git/,
		// non-source files and excluded globs are never read.
		"caller FindByCustomerAsync src/Controllers/ReportsController.cs:4-7",
		"definition saveAudit web/audit.ts:1-3",
	}
	slices.Sort(lines)
	slices.Sort(want2)
	if !reflect.DeepEqual(lines, want2) {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(lines, "\n  "), strings.Join(want2, "\n  "))
	}
	// Deterministic: the same input gives the same snippets in the same order.
	again, _ := Scan(repo(), want, Limits{MaxFileBytes: 2000, Exclude: []string{"*.g.cs"}})
	if !reflect.DeepEqual(got, again) {
		t.Error("scan is not deterministic")
	}
	for _, s := range got {
		if s.Kind == "definition" && s.Symbol == "saveAudit" && !strings.Contains(s.Text, "await db.insert(event);") {
			t.Errorf("definition text %q lacks the body", s.Text)
		}
	}
}

func TestScan_LimitsFiles(t *testing.T) {
	got, err := Scan(repo(), Want{Used: map[Family]map[string]bool{JS: {"saveAudit": true}}}, Limits{MaxFiles: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > 1 {
		t.Errorf("MaxFiles 1 read more than one source file: %+v", got)
	}
}

func itoa(n int) string {
	s := ""
	if n == 0 {
		return "0"
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

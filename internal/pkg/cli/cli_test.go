package cli

import (
	"bytes"
	"testing"

	"github.com/adamluzsi/testcase"
)

func TestParse(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Describe(`Parse`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) (*Input, error) {
			name := t.I(`name`).(string)
			args := t.I(`args`).([]string)
			return Parse(name, args)
		}

		s.When(`no args are provided`, func(s *testcase.Spec) {
			s.Let(`args`, func(t *testcase.T) interface{} {
				return []string{}
			})

			s.LetValue(`name`, "gosrv")

			s.Then(`it returns default input set with no error`, func(t *testcase.T) {
				inp, err := subject(t)

				if err != nil {
					t.Errorf("got err = %v, want err <nil>", err)
				}

				if inp == nil {
					t.Errorf("got = %v, want = <non-nil>", inp)
				}
			})
		})

		s.When(`invalid args are provided`, func(s *testcase.Spec) {
			s.Let(`args`, func(t *testcase.T) interface{} {
				return []string{"-some-unknown-flag"}
			})

			s.LetValue(`name`, "gosrv")

			s.Then(`it returns non nil error`, func(t *testcase.T) {
				inp, err := subject(t)

				if err == nil {
					t.Errorf("got err = %v, want err <non-nil>", err)
				}

				if inp != nil {
					t.Errorf("got = %v, want = <nil>", inp)
				}
			})
		})

		s.When(`help flag is provided`, func(s *testcase.Spec) {
			s.Let(`args`, func(t *testcase.T) interface{} {
				return []string{"-help"}
			})

			s.LetValue(`name`, "gosrv")

			s.Then(`it returns input set with help true`, func(t *testcase.T) {
				inp, err := subject(t)

				if err != nil {
					t.Errorf("got err = %v, want err <nil>", err)
				}

				if inp == nil {
					t.Fatalf("got = %v, want = <non-nil>", inp)
				}

				if inp.Help == false {
					t.Errorf("got help = %v, want help = %v", inp.Help, true)
				}
			})
		})

		s.When(`quiet flag is provided`, func(s *testcase.Spec) {
			s.Let(`args`, func(t *testcase.T) interface{} {
				return []string{"-quiet"}
			})

			s.LetValue(`name`, "gosrv")

			s.Then(`it returns input set with quiet true`, func(t *testcase.T) {
				inp, err := subject(t)

				if err != nil {
					t.Errorf("got err = %v, want err <nil>", err)
				}

				if inp == nil {
					t.Fatalf("got = %v, want = <non-nil>", inp)
				}

				if inp.Quiet == false {
					t.Errorf("got quiet = %v, want quiet = %v", inp.Quiet, true)
				}
			})
		})

		s.When(`dir flag is provided`, func(s *testcase.Spec) {
			s.Let(`args`, func(t *testcase.T) interface{} {
				dir := t.I(`dir`).(string)
				return []string{"-dir", dir}
			})

			s.LetValue(`name`, "gosrv")
			s.LetValue(`dir`, "./some/dir")

			s.Then(`it returns input set with provided dir`, func(t *testcase.T) {
				inp, err := subject(t)

				if err != nil {
					t.Errorf("got err = %v, want err <nil>", err)
				}

				if inp == nil {
					t.Fatalf("got = %v, want = <non-nil>", inp)
				}

				wantDir := t.I(`dir`).(string)

				if inp.Dir != wantDir {
					t.Errorf("got dir = %v, want dir = %v", inp.Dir, wantDir)
				}
			})
		})

		s.When(`host flag is provided`, func(s *testcase.Spec) {
			s.Let(`args`, func(t *testcase.T) interface{} {
				host := t.I(`host`).(string)
				return []string{"-host", host}
			})

			s.LetValue(`name`, "gosrv")
			s.LetValue(`host`, "192.168.1.6")

			s.Then(`it returns input set with provided host`, func(t *testcase.T) {
				inp, err := subject(t)

				if err != nil {
					t.Errorf("got err = %v, want err <nil>", err)
				}

				if inp == nil {
					t.Fatalf("got = %v, want = <non-nil>", inp)
				}

				wantHost := t.I(`host`).(string)

				if inp.Host != wantHost {
					t.Errorf("got host = %v, want host = %v", inp.Host, wantHost)
				}
			})
		})

		s.When(`port flag is provided`, func(s *testcase.Spec) {
			s.Let(`args`, func(t *testcase.T) interface{} {
				port := t.I(`port`).(string)
				return []string{"-port", port}
			})

			s.LetValue(`name`, "gosrv")
			s.LetValue(`port`, "8998")

			s.Then(`it returns input set with provided port`, func(t *testcase.T) {
				inp, err := subject(t)

				if err != nil {
					t.Errorf("got err = %v, want err <nil>", err)
				}

				if inp == nil {
					t.Fatalf("got = %v, want = <non-nil>", inp)
				}

				wantPort := t.I(`port`).(string)

				if inp.Port != wantPort {
					t.Errorf("got port = %v, want port = %v", inp.Port, wantPort)
				}
			})
		})

		s.When(`key flag is provided but cert flag is not`, func(s *testcase.Spec) {
			s.Let(`args`, func(t *testcase.T) interface{} {
				key := t.I(`key`).(string)
				return []string{"-key", key}
			})

			s.LetValue(`name`, "gosrv")
			s.LetValue(`key`, "/path/to/key")

			s.Then(`it returns non nil error`, func(t *testcase.T) {
				inp, err := subject(t)

				if err == nil {
					t.Errorf("got err = %v, want err <non-nil>", err)
				}

				if inp != nil {
					t.Errorf("got = %v, want = <nil>", inp)
				}
			})
		})

		s.When(`cert flag is provided but key flag is not`, func(s *testcase.Spec) {
			s.Let(`args`, func(t *testcase.T) interface{} {
				cert := t.I(`cert`).(string)
				return []string{"-cert", cert}
			})

			s.LetValue(`name`, "gosrv")
			s.LetValue(`cert`, "/path/to/cert")

			s.Then(`it returns non nil error`, func(t *testcase.T) {
				inp, err := subject(t)

				if err == nil {
					t.Errorf("got err = %v, want err <non-nil>", err)
				}

				if inp != nil {
					t.Errorf("got = %v, want = <nil>", inp)
				}
			})
		})

		s.When(`both key and cert flags are provided`, func(s *testcase.Spec) {
			s.Let(`args`, func(t *testcase.T) interface{} {
				key := t.I(`key`).(string)
				cert := t.I(`cert`).(string)
				return []string{"-key", key, "-cert", cert}
			})

			s.LetValue(`name`, "gosrv")
			s.LetValue(`key`, "/path/to/key")
			s.LetValue(`cert`, "/path/to/cert")

			s.Then(`it returns input set with provided key and cert`, func(t *testcase.T) {
				inp, err := subject(t)

				if err != nil {
					t.Errorf("got err = %v, want err <nil>", err)
				}

				if inp == nil {
					t.Fatalf("got = %v, want = <non-nil>", inp)
				}

				wantKey := t.I(`key`).(string)
				wantCert := t.I(`cert`).(string)

				if inp.KeyFile != wantKey {
					t.Errorf("got key file = %v, want key file = %v", inp.KeyFile, wantKey)
				}

				if inp.CertFile != wantCert {
					t.Errorf("got cert file = %v, want cert file = %v", inp.CertFile, wantCert)
				}
			})
		})
	})
}

func TestWriteHelp(t *testing.T) {
	var buf bytes.Buffer

	WriteHelp(&buf, "some-name")

	b := buf.Bytes()

	if len(b) == 0 {
		t.Errorf("did not write anything on buffer")
	}

	if !bytes.Contains(b, []byte("some-name")) {
		t.Errorf("help output does not contain name")
	}
}

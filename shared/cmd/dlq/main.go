// Command dlq inspecciona y reprocesa la dead letter queue de JetStream.
//
//	dlq list                        lista los mensajes en la DLQ
//	dlq show    <seq>               muestra headers y payload de un mensaje
//	dlq replay  <seq>|all [-consumer X]  republica al subject original y lo borra de la DLQ
//	dlq discard <seq>               borra un mensaje de la DLQ sin reprocesarlo
//
// Ojo: replay republica al subject original, así que le llega a TODOS los
// consumers de ese subject, no solo al que falló. Los consumers deben ser idempotentes.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"

	"github.com/juantevez/my-ig/shared/dlq"
	"github.com/nats-io/nats.go"
)

func main() {
	url := flag.String("server", envOr("NATS_URL", nats.DefaultURL), "NATS server URL")
	consumer := flag.String("consumer", "", "replay all: solo mensajes de este consumer")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "uso: dlq [-server url] [-consumer name] list | show <seq> | replay <seq>|all | discard <seq>")
	}
	flag.Parse()

	nc, err := nats.Connect(*url)
	if err != nil {
		fail("connect: %v", err)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		fail("jetstream: %v", err)
	}

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	switch args[0] {
	case "list":
		err = list(js)
	case "show":
		err = withSeq(args, func(seq uint64) error { return show(js, seq) })
	case "replay":
		if len(args) == 2 && args[1] == "all" {
			err = replayAll(js, *consumer)
		} else {
			err = withSeq(args, func(seq uint64) error { return replay(js, seq) })
		}
	case "discard":
		err = withSeq(args, func(seq uint64) error {
			if err := js.DeleteMsg(dlq.StreamName, seq); err != nil {
				return err
			}
			fmt.Printf("seq %d descartado\n", seq)
			return nil
		})
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fail("%s: %v", args[0], err)
	}
}

// messages recorre los mensajes vivos de la DLQ en orden.
func messages(js nats.JetStreamContext, fn func(*nats.RawStreamMsg) error) error {
	info, err := js.StreamInfo(dlq.StreamName)
	if err != nil {
		return fmt.Errorf("stream %s: %w", dlq.StreamName, err)
	}
	if info.State.Msgs == 0 {
		return nil
	}
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq; seq++ {
		m, err := js.GetMsg(dlq.StreamName, seq)
		if errors.Is(err, nats.ErrMsgNotFound) {
			continue // borrado (replay/discard)
		}
		if err != nil {
			return fmt.Errorf("get seq %d: %w", seq, err)
		}
		if err := fn(m); err != nil {
			return err
		}
	}
	return nil
}

func list(js nats.JetStreamContext) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "SEQ\tSUBJECT\tCONSUMER\tDELIVERIES\tFAILED AT\tERROR")
	n := 0
	err := messages(js, func(m *nats.RawStreamMsg) error {
		n++
		h := m.Header
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n", m.Sequence,
			h.Get(dlq.HeaderOriginalSubject), h.Get(dlq.HeaderConsumer),
			h.Get(dlq.HeaderDeliveries), h.Get(dlq.HeaderFailedAt), truncate(h.Get(dlq.HeaderError), 80))
		return nil
	})
	w.Flush()
	fmt.Printf("%d mensaje(s) en la DLQ\n", n)
	return err
}

func show(js nats.JetStreamContext, seq uint64) error {
	m, err := js.GetMsg(dlq.StreamName, seq)
	if err != nil {
		return err
	}
	for k, v := range m.Header {
		fmt.Printf("%s: %s\n", k, v[0])
	}
	fmt.Printf("\n%s\n", m.Data)
	return nil
}

func replay(js nats.JetStreamContext, seq uint64) error {
	m, err := js.GetMsg(dlq.StreamName, seq)
	if err != nil {
		return err
	}
	return republish(js, m)
}

func replayAll(js nats.JetStreamContext, consumer string) error {
	n := 0
	err := messages(js, func(m *nats.RawStreamMsg) error {
		if consumer != "" && m.Header.Get(dlq.HeaderConsumer) != consumer {
			return nil
		}
		n++
		return republish(js, m)
	})
	fmt.Printf("%d mensaje(s) reprocesados\n", n)
	return err
}

// republish manda el payload original a su subject y, con el ack, lo borra de la DLQ.
func republish(js nats.JetStreamContext, m *nats.RawStreamMsg) error {
	subject := m.Header.Get(dlq.HeaderOriginalSubject)
	if subject == "" {
		return fmt.Errorf("seq %d: sin header %s", m.Sequence, dlq.HeaderOriginalSubject)
	}

	out := nats.NewMsg(subject)
	out.Data = m.Data
	for k, v := range m.Header {
		if len(k) >= 4 && k[:4] == "Dlq-" {
			continue
		}
		out.Header[k] = v
	}
	if _, err := js.PublishMsg(out); err != nil {
		return fmt.Errorf("seq %d: publish %s: %w", m.Sequence, subject, err)
	}
	if err := js.DeleteMsg(dlq.StreamName, m.Sequence); err != nil {
		return fmt.Errorf("seq %d: republicado pero no se pudo borrar de la DLQ: %w", m.Sequence, err)
	}
	fmt.Printf("seq %d → %s\n", m.Sequence, subject)
	return nil
}

func withSeq(args []string, fn func(uint64) error) error {
	if len(args) != 2 {
		return errors.New("falta <seq>")
	}
	seq, err := strconv.ParseUint(args[1], 10, 64)
	if err != nil {
		return fmt.Errorf("seq inválido %q", args[1])
	}
	return fn(seq)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "dlq: "+format+"\n", a...)
	os.Exit(1)
}

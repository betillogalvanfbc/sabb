package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"
)

// sanitizeKey elimina cualquier carácter de espacio (espacios, tabulaciones,
// saltos de línea) para evitar que la cabecera Authorization falle.
func sanitizeKey(k string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, k)
}

// ProgramFetcher define una interfaz común para las plataformas.
// Retorna el número de programas procesados y un error en caso de fallo.

type ProgramFetcher interface {
	Fetch(ctx context.Context, apiKey string, out io.Writer) (int, error)
}

/*************************
 * HackerOne implementation
 *************************/

type hackerOneFetcher struct{}

type hackerOneProgramsPage struct {
	Data []struct {
		Attributes struct {
			Handle         string `json:"handle"`
			OffersBounties bool   `json:"offers_bounties"`
		} `json:"attributes"`
	} `json:"data"`
}

type hackerOneScopePage struct {
	Data []struct {
		Attributes struct {
			EligibleForBounty bool   `json:"eligible_for_bounty"`
			AssetIdentifier   string `json:"asset_identifier"`
		} `json:"attributes"`
	} `json:"data"`
}

func (h hackerOneFetcher) Fetch(ctx context.Context, apiKey string, out io.Writer) (int, error) {
	// Cliente con timeout más generoso para evitar timeouts prematuros
	client := &http.Client{Timeout: 30 * time.Second}

	// Extraer username y apiKey del string combinado
	parts := strings.SplitN(apiKey, ":", 2)
	if len(parts) != 2 {
		return 0, fmt.Errorf("formato de credenciales inválido, debe ser username:apikey")
	}
	username, key := parts[0], parts[1]
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + key))
	processed := 0

	for page := 1; ; page++ {
		select {
		case <-ctx.Done():
			return processed, ctx.Err()
		default:
		}

		url := fmt.Sprintf("https://api.hackerone.com/v1/hackers/programs?page[number]=%d&page[size]=100", page)
		body, err := doRequestWithRetry(ctx, client, url, auth)
		if err != nil {
			return processed, fmt.Errorf("programs page request failed: %w", err)
		}

		var pg hackerOneProgramsPage
		if err := safeUnmarshal(body, &pg); err != nil {
			return processed, err
		}

		if len(pg.Data) == 0 {
			break // no more pages
		}

		for _, d := range pg.Data {
			if !d.Attributes.OffersBounties {
				continue
			}
			handle := d.Attributes.Handle
			fmt.Printf("Procesando: %s\n", handle)

			assets, err := h.fetchEligibleAssets(ctx, client, auth, handle)
			if err != nil {
				// devolvemos error: usuario pidió que solo salga el error
				return processed, fmt.Errorf("handle %s failed: %w", handle, err)
			}
			for _, asset := range assets {
				fmt.Fprintln(out, asset)
			}
			processed++
		}
	}

	return processed, nil
}

// doRequestWithRetry intenta la solicitud hasta 3 veces con un delay exponencial
func doRequestWithRetry(ctx context.Context, client *http.Client, url, auth string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			// Espera exponencial: 1s, 2s, 4s
			delay := time.Duration(1<<uint(attempt)) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		body, err := doRequest(ctx, client, url, auth)
		if err == nil {
			return body, nil
		}
		lastErr = err

		// Si el error no es por timeout, no reintentamos
		if !strings.Contains(err.Error(), "deadline exceeded") {
			return nil, err
		}
	}
	return nil, fmt.Errorf("después de 3 intentos: %w", lastErr)
}

func (h hackerOneFetcher) fetchEligibleAssets(ctx context.Context, client *http.Client, auth, handle string) ([]string, error) {
	url := fmt.Sprintf("https://api.hackerone.com/v1/hackers/programs/%s/structured_scopes", handle)
	body, err := doRequestWithRetry(ctx, client, url, auth)
	if err != nil {
		return nil, err
	}

	var pg hackerOneScopePage
	if err := safeUnmarshal(body, &pg); err != nil {
		return nil, err
	}

	var assets []string
	for _, d := range pg.Data {
		if d.Attributes.EligibleForBounty {
			assets = append(assets, d.Attributes.AssetIdentifier)
		}
	}
	return assets, nil
}

// doRequest centraliza la lógica HTTP con manejo de errores, timeout y códigos de estado.
func doRequest(ctx context.Context, client *http.Client, url, auth string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+auth)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("API unavailable: %s", resp.Status)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API returned error %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// safeUnmarshal incluye recuperación de panic por JSON inválido.
func safeUnmarshal(data []byte, v interface{}) error {
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			if !ok {
				err = errors.New("panic")
			}
			log.Printf("panic recuperado: %v", err)
		}
	}()
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("JSON decode failed: %w", err)
	}
	return nil
}

/**********************************
 * Placeholders para otras plataformas
 **********************************/

type notImplementedFetcher struct{ name string }

func (n notImplementedFetcher) Fetch(context.Context, string, io.Writer) (int, error) {
	return 0, fmt.Errorf("fetcher para %s aún no implementado", n.name)
}

/*****************
 * Función principal
 *****************/

func main() {
	programFlag := flag.String("program", "hackerone", "Plataforma(s) separadas por comas: hackerone,intigriti,bugcrowd")
	username := flag.String("username", "", "HackerOne username")
	apiKey := flag.String("apikey", "", "API key")
	outputFile := flag.String("output", "programasguardado.txt", "Archivo de salida")
	timeout := flag.Duration("timeout", 30*time.Second, "Timeout total de ejecución")
	flag.Parse()

	if *apiKey == "" {
		log.Fatal("apikey es obligatorio")
	}
	if *username == "" {
		log.Fatal("username es obligatorio para HackerOne")
	}

	cleanKey := sanitizeKey(*apiKey)
	cleanUsername := sanitizeKey(*username)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	f, err := os.OpenFile(*outputFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("no se pudo abrir %s: %v", *outputFile, err)
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	defer writer.Flush()

	fetchers := map[string]ProgramFetcher{
		"hackerone": hackerOneFetcher{},
		"intigriti": notImplementedFetcher{"Intigriti"},
		"bugcrowd":  notImplementedFetcher{"Bugcrowd"},
	}

	total := 0

	for _, p := range strings.Split(*programFlag, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		fetcher, ok := fetchers[p]
		if !ok {
			log.Printf("programa desconocido: %s", p)
			continue
		}
		var credentials string
		if p == "hackerone" {
			credentials = cleanUsername + ":" + cleanKey
		} else {
			credentials = cleanKey
		}
		cnt, err := fetcher.Fetch(ctx, credentials, writer)
		if err != nil {
			// Imprime sólo el error y termina — petición del usuario
			log.Fatalf("ERROR: %v", err)
		}
		total += cnt
	}

	fmt.Printf("Total de programas procesados: %d\n", total)
}

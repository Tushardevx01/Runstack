package api

import (
	"context"
	"github.com/Tushardevx01/runstack/internal/node"

	"net/http"
	"strings"
)

type AuthManager struct {
	operatorToken string
	agentToken    string
}

func NewAuthManager(operatorToken, agentToken string) *AuthManager {
	return &AuthManager{
		operatorToken: operatorToken,
		agentToken:    agentToken,
	}
}

func (m *AuthManager) extractToken(r *http.Request) (string, int) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", http.StatusUnauthorized
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
		return "", http.StatusUnauthorized
	}

	token := parts[1]
	if token == "" {
		return "", http.StatusUnauthorized
	}
	if token != m.operatorToken && token != m.agentToken {
		return "", http.StatusUnauthorized
	}

	return token, http.StatusOK
}

func (m *AuthManager) RequireOperator(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m.operatorToken == "" && m.agentToken == "" {
			// Auth disabled? Actually, auth should always be required now.
		}

		token, status := m.extractToken(r)
		if status != http.StatusOK {
			http.Error(w, "Unauthorized", status)
			return
		}

		if token != m.operatorToken {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

func (m *AuthManager) RequireAgent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, status := m.extractToken(r)
		if status != http.StatusOK {
			http.Error(w, "Unauthorized", status)
			return
		}

		if token != m.agentToken {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next(w, r)
	}
}

func (m *AuthManager) RequireNodeAuth(nodeReg *node.Registry, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, status := m.extractToken(r)
		if status != http.StatusOK {
			http.Error(w, "Unauthorized", status)
			return
		}

		n, ok := nodeReg.GetByToken(token)
		if !ok {
			http.Error(w, "Unauthorized: invalid node token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), "node_id", n.ID)
		next(w, r.WithContext(ctx))
	}
}

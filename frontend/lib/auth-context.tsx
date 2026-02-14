"use client";

import {
  createContext,
  useContext,
  useEffect,
  useState,
  useCallback,
  ReactNode,
} from "react";
import {
  signIn as amplifySignIn,
  signUp as amplifySignUp,
  confirmSignUp as amplifyConfirmSignUp,
  signOut as amplifySignOut,
  updatePassword as amplifyUpdatePassword,
  updateUserAttribute as amplifyUpdateUserAttribute,
  confirmUserAttribute as amplifyConfirmUserAttribute,
  fetchAuthSession,
  getCurrentUser,
} from "aws-amplify/auth";
import { configureAmplify } from "./amplify-config";

interface AuthContextType {
  user: { userId: string; email: string } | null;
  loading: boolean;
  signIn: (email: string, password: string) => Promise<void>;
  signUp: (email: string, password: string) => Promise<void>;
  confirmSignUp: (email: string, code: string) => Promise<void>;
  signOut: () => Promise<void>;
  getAccessToken: () => Promise<string | null>;
  updatePassword: (oldPassword: string, newPassword: string) => Promise<void>;
  updateEmail: (newEmail: string) => Promise<void>;
  confirmEmailUpdate: (code: string) => Promise<void>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<{ userId: string; email: string } | null>(
    null
  );
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    configureAmplify();
    checkUser();
  }, []);

  const checkUser = useCallback(async () => {
    try {
      const currentUser = await getCurrentUser();
      const session = await fetchAuthSession();
      const email =
        session.tokens?.idToken?.payload?.email?.toString() || "";
      setUser({ userId: currentUser.userId, email });
    } catch {
      setUser(null);
    } finally {
      setLoading(false);
    }
  }, []);

  const signIn = useCallback(async (email: string, password: string) => {
    const result = await amplifySignIn({ username: email, password });
    if (result.isSignedIn) {
      await checkUser();
    }
  }, [checkUser]);

  const signUp = useCallback(async (email: string, password: string) => {
    await amplifySignUp({
      username: email,
      password,
      options: { userAttributes: { email } },
    });
  }, []);

  const confirmSignUp = useCallback(async (email: string, code: string) => {
    await amplifyConfirmSignUp({ username: email, confirmationCode: code });
  }, []);

  const signOut = useCallback(async () => {
    await amplifySignOut();
    setUser(null);
  }, []);

  const getAccessToken = useCallback(async (): Promise<string | null> => {
    try {
      const session = await fetchAuthSession();
      return session.tokens?.idToken?.toString() || null;
    } catch {
      return null;
    }
  }, []);

  const updatePassword = useCallback(async (oldPassword: string, newPassword: string) => {
    await amplifyUpdatePassword({ oldPassword, newPassword });
  }, []);

  const updateEmail = useCallback(async (newEmail: string) => {
    await amplifyUpdateUserAttribute({
      userAttribute: { attributeKey: "email", value: newEmail },
    });
  }, []);

  const confirmEmailUpdate = useCallback(async (code: string) => {
    await amplifyConfirmUserAttribute({
      userAttributeKey: "email",
      confirmationCode: code,
    });
    await checkUser();
  }, [checkUser]);

  return (
    <AuthContext.Provider
      value={{ user, loading, signIn, signUp, confirmSignUp, signOut, getAccessToken, updatePassword, updateEmail, confirmEmailUpdate }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}
